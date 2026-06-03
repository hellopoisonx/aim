#!/usr/bin/env bash
# AIM 生产部署自动化。
#
# 约定：
#   - env 文件默认 /etc/aim/aim.env（由 deploy/env/prod.example.env 拷贝而来）
#   - config 目录默认 /etc/aim/config（由 deploy/config/prod.example/ 拷贝而来）
#   - 备份目录默认 /var/backups/aim，deploy.sh 的 up / down / rollback 都会先留快照
#   - 反向代理 / TLS 终止由宿主或集群侧（nginx / Caddy / 云 LB / k8s ingress）负责，
#     AIM 容器不内置反代。
#
# 用法：deploy/deploy.sh <子命令> [参数...]
# 跑 deploy/deploy.sh help 查看完整说明。

set -euo pipefail

# === 路径与默认值 ============================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

AIM_COMPOSE_DIR="${AIM_COMPOSE_DIR:-$REPO_ROOT/deploy/compose}"
COMPOSE_BASE="$AIM_COMPOSE_DIR/base.yaml"
COMPOSE_PROD="$AIM_COMPOSE_DIR/prod.yaml"
CONFIG_PROD_EXAMPLE="$REPO_ROOT/deploy/config/prod.example"
ENV_PROD_EXAMPLE="$REPO_ROOT/deploy/env/prod.example.env"

AIM_ENV_FILE="${AIM_ENV_FILE:-/etc/aim/aim.env}"
AIM_CONFIG_DIR="${AIM_CONFIG_DIR:-/etc/aim/config}"
AIM_BACKUP_DIR="${AIM_BACKUP_DIR:-/var/backups/aim}"

# 视为“需要 root 写权限”的子命令。
NEED_ROOT_CMDS=(init up down restart backup rollback)
# 部署必须存在的服务配置。
REQUIRED_CONFIG_FILES=(auth.yaml logic.yaml core.yaml gateway-api.yaml attachment.yaml data_parsing.yaml seaweed-s3.json)
# 一键迁移服务（按顺序跑，依赖图来自 deploy/compose/base.yaml）。
INIT_SERVICES=(auth-migrate logic-migrate kafka-init seaweed-bucket-init)

# === 输出样式 ================================================================

if [[ -t 1 ]] && command -v tput >/dev/null 2>&1; then
  C_RED=$'\033[0;31m'
  C_GREEN=$'\033[0;32m'
  C_YELLOW=$'\033[0;33m'
  C_BLUE=$'\033[0;34m'
  C_BOLD=$'\033[1m'
  C_RESET=$'\033[0m'
else
  C_RED='' C_GREEN='' C_YELLOW='' C_BLUE='' C_BOLD='' C_RESET=''
fi

log()  { printf '%b ==>%b %s\n' "$C_BLUE" "$C_RESET" "$*"; }
ok()   { printf '%b  ✔%b %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf '%b  !!%b %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
err()  { printf '%b  ✘%b %s\n' "$C_RED" "$C_RESET" "$*" >&2; }
die()  { err "$*"; exit 1; }
section() { printf '\n%b%s%b\n' "$C_BOLD" "$*" "$C_RESET"; }

# === 工具函数 =================================================================

compose_cmd() {
  docker compose --env-file "$AIM_ENV_FILE" \
    -f "$COMPOSE_BASE" \
    -f "$COMPOSE_PROD" \
    "$@"
}

need_root() {
  local cmd="$1"
  for c in "${NEED_ROOT_CMDS[@]}"; do
    [[ "$c" == "$cmd" ]] && return 0
  done
  return 1
}

require_root() {
  if [[ $EUID -ne 0 ]]; then
    die "请用 root 或 sudo 运行：sudo $0 $*"
  fi
}

require_docker() {
  command -v docker >/dev/null 2>&1 || die "docker 未安装或不在 PATH"
  docker compose version >/dev/null 2>&1 || die "docker compose v2 不可用（需要 Docker 20.10+ / Compose plugin）"
  ok "docker / compose 可用"
}

require_repo_layout() {
  [[ -f "$COMPOSE_BASE" ]] || die "找不到 $COMPOSE_BASE；请在仓库根目录执行，或设置 AIM_COMPOSE_DIR 指向正确路径"
  [[ -f "$COMPOSE_PROD" ]] || die "找不到 $COMPOSE_PROD；请在仓库根目录执行，或设置 AIM_COMPOSE_DIR 指向正确路径"
  [[ -d "$CONFIG_PROD_EXAMPLE" ]] || die "找不到 $CONFIG_PROD_EXAMPLE"
  [[ -f "$ENV_PROD_EXAMPLE" ]] || die "找不到 $ENV_PROD_EXAMPLE"
}

require_env_file() {
  [[ -f "$AIM_ENV_FILE" ]] || die "env 文件不存在: $AIM_ENV_FILE；先运行 '$0 init' 或手动准备。"
  ok "env 文件存在: $AIM_ENV_FILE"
}

require_config_dir() {
  [[ -d "$AIM_CONFIG_DIR" ]] || die "config 目录不存在: $AIM_CONFIG_DIR；先运行 '$0 init'。"
  local missing=()
  for f in "${REQUIRED_CONFIG_FILES[@]}"; do
    [[ -f "$AIM_CONFIG_DIR/$f" ]] || missing+=("$f")
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    die "config 目录缺少文件: ${missing[*]}；先运行 '$0 init' 或手动补齐。"
  fi
  ok "config 目录完整: $AIM_CONFIG_DIR"
}

env_check_placeholders() {
  # 只检查非注释行，避免 prod.example.env 里的 “CHANGE_ME” 注释误报。
  local hits
  hits="$(grep -nE '^[[:space:]]*[^#[:space:]].*CHANGE_ME' "$AIM_ENV_FILE" || true)"
  if [[ -n "$hits" ]]; then
    warn "env 文件中检测到未替换的 CHANGE_ME 占位符："
    printf '%s\n' "$hits" | sed 's/^/    /'
    die "请先编辑 $AIM_ENV_FILE 替换所有 CHANGE_ME。"
  fi
  ok "env 文件无 CHANGE_ME 占位符"
}

env_check_prod_markers() {
  # 防止把开发态配置（POSTGRES_PASSWORD=password / *_ACCESS_KEY=aim）带到生产。
  local suspicious
  suspicious="$(grep -inE '^[[:space:]]*[^#[:space:]].*(password=password|access[-_]key=aim($|[^a-z]))$' "$AIM_ENV_FILE" || true)"
  if [[ -n "$suspicious" ]]; then
    warn "env 文件存在开发态默认值，请确认是否需要替换："
    printf '%s\n' "$suspicious" | sed 's/^/    /'
    if [[ "${AIM_ALLOW_DEV_DEFAULTS:-0}" != "1" ]]; then
      die "确认无误请设置 AIM_ALLOW_DEV_DEFAULTS=1 后重试。"
    fi
    warn "AIM_ALLOW_DEV_DEFAULTS=1 已设置，继续。"
  fi
}

backup_id_now() {
  date -u +%Y%m%d-%H%M%S
}

run_backup() {
  local label="${1:-manual}"
  local ts dest
  ts="$(backup_id_now)"
  dest="$AIM_BACKUP_DIR/${ts}${label:+-$label}"
  mkdir -p "$dest/config"
  if [[ -d "$AIM_CONFIG_DIR" ]]; then
    cp -a "$AIM_CONFIG_DIR/." "$dest/config/"
  else
    warn "config 目录不存在，跳过 config 备份: $AIM_CONFIG_DIR"
  fi
  if [[ -f "$AIM_ENV_FILE" ]]; then
    cp -a "$AIM_ENV_FILE" "$dest/aim.env"
    chmod 600 "$dest/aim.env"
  else
    warn "env 文件不存在，跳过 env 备份: $AIM_ENV_FILE"
  fi
  (cd "$dest" && find . -type f -print0 | sort -z | xargs -0 sha256sum > manifest.sha256)
  printf '%s\n' "$dest" > "$AIM_BACKUP_DIR/.latest"
  ok "已备份到 $dest"
}

list_backups() {
  if [[ ! -d "$AIM_BACKUP_DIR" ]]; then
    warn "备份目录不存在: $AIM_BACKUP_DIR"
    return 0
  fi
  local found=0
  while IFS= read -r d; do
    [[ -z "$d" ]] && continue
    found=1
    local label
    label="$( [[ -f "$d/aim.env" ]] && echo 'env+config' || echo 'partial' )"
    printf '  %s  %s\n' "$(basename "$d")" "$label"
  done < <(find "$AIM_BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d ! -name '.*' | sort)
  if [[ $found -eq 0 ]]; then
    warn "备份目录为空: $AIM_BACKUP_DIR"
  fi
}

resolve_backup() {
  local id="${1:-}"
  local dest
  if [[ -z "$id" ]]; then
    if [[ -f "$AIM_BACKUP_DIR/.latest" ]]; then
      dest="$(cat "$AIM_BACKUP_DIR/.latest")"
      warn "未指定备份 id，使用 .latest: $dest"
    else
      die "没有可用备份；用法：$0 rollback <YYYYmmdd-HHMMSS[-label]>"
    fi
  else
    dest="$AIM_BACKUP_DIR/$id"
  fi
  [[ -d "$dest" ]] || die "备份目录不存在: $dest"
  printf '%s\n' "$dest"
}

# === 子命令 ==================================================================

cmd_preflight() {
  section "preflight: 校验部署前置条件"
  require_repo_layout
  require_docker
  require_env_file
  require_config_dir
  env_check_placeholders
  env_check_prod_markers
  ok "preflight 通过 ✅"
}

cmd_init() {
  require_root "$@"
  section "init: 引导 $AIM_ENV_FILE 与 $AIM_CONFIG_DIR"
  require_repo_layout
  if { [[ -e "$AIM_ENV_FILE" ]] || [[ -d "$AIM_CONFIG_DIR" ]]; } && [[ "${AIM_INIT_FORCE:-0}" != "1" ]]; then
    die "已存在 $AIM_ENV_FILE 或 $AIM_CONFIG_DIR；如确需重建请设置 AIM_INIT_FORCE=1 后重试。"
  fi
  mkdir -p "$AIM_CONFIG_DIR"
  cp -a "$CONFIG_PROD_EXAMPLE/." "$AIM_CONFIG_DIR/"
  cp -a "$ENV_PROD_EXAMPLE" "$AIM_ENV_FILE"
  chmod 600 "$AIM_ENV_FILE"
  ok "/etc/aim 已就绪："
  printf '  - env:    %s (chmod 600)\n' "$AIM_ENV_FILE"
  printf '  - config: %s/\n' "$AIM_CONFIG_DIR"
  cat <<'NEXT'

下一步：
  1) 编辑 $AIM_ENV_FILE，替换所有 CHANGE_ME_*、设置真实域名与强随机密钥。
  2) 编辑 $AIM_CONFIG_DIR/*.yaml，确认 etcd / postgres / seaweed 集群信息正确。
  3) sudo deploy/deploy.sh preflight   # 校验
  4) sudo deploy/deploy.sh up          # 启动
NEXT
}

cmd_config_check() {
  cmd_preflight
}

cmd_migrate() {
  section "migrate: 跑 PostgreSQL 迁移 + Kafka topic + SeaweedFS bucket"
  cmd_preflight
  local targets=("$@")
  if [[ ${#targets[@]} -eq 0 ]]; then
    targets=("${INIT_SERVICES[@]}")
  fi
  for svc in "${targets[@]}"; do
    log " -> $svc"
    if ! compose_cmd up --force-recreate --abort-on-container-exit --exit-code-from "$svc" "$svc"; then
      die "$svc 失败；请查看上方日志定位问题。"
    fi
  done
  ok "migrate 完成 ✅"
}

cmd_up() {
  require_root "$@"
  section "up: 部署生产栈"
  cmd_preflight
  run_backup "before-up"
  cmd_migrate
  log " -> docker compose up -d --build $*"
  compose_cmd up -d --build "$@"
  cmd_status
}

cmd_down() {
  require_root "$@"
  section "down: 停止生产栈"
  # down 不强制 preflight，避免栈已坏时无法收尾。
  require_repo_layout
  require_docker
  require_env_file || true
  if [[ -d "$AIM_CONFIG_DIR" ]]; then
    run_backup "before-down"
  fi
  log " -> docker compose down $*"
  compose_cmd down "$@"
  ok "down 完成"
}

cmd_restart() {
  require_root "$@"
  section "restart: down + up"
  cmd_down
  cmd_up "$@"
}

cmd_status() {
  log "容器状态："
  compose_cmd ps
}

cmd_logs() {
  compose_cmd logs "$@"
}

cmd_backup() {
  require_root "$@"
  section "backup: 备份 /etc/aim"
  run_backup "${1:-manual}"
}

cmd_rollback() {
  require_root "$@"
  section "rollback: 从备份恢复 /etc/aim"
  local dest
  dest="$(resolve_backup "${1:-}")"
  if [[ -f "$dest/manifest.sha256" ]]; then
    log "校验备份完整性..."
    (cd "$dest" && sha256sum -c manifest.sha256) || die "备份校验失败：$dest"
  else
    warn "备份 $dest 没有 manifest.sha256，跳过完整性校验"
  fi
  if [[ "${AIM_ROLLBACK_FORCE:-0}" != "1" ]]; then
    warn "即将用 $dest 覆盖当前 $AIM_CONFIG_DIR 与 $AIM_ENV_FILE。"
    warn "确认请设置 AIM_ROLLBACK_FORCE=1 后重试，或使用 'list-backups' 选一个更新的 id。"
    die "rollback 取消。"
  fi
  run_backup "before-rollback"
  rm -rf "$AIM_CONFIG_DIR"
  mkdir -p "$AIM_CONFIG_DIR"
  if [[ -d "$dest/config" ]]; then
    cp -a "$dest/config/." "$AIM_CONFIG_DIR/"
  fi
  if [[ -f "$dest/aim.env" ]]; then
    cp -a "$dest/aim.env" "$AIM_ENV_FILE"
    chmod 600 "$AIM_ENV_FILE"
  fi
  ok "已恢复 $dest"
  log "重新启动栈..."
  cmd_up
}

cmd_list_backups() {
  section "rollback 候选快照（$AIM_BACKUP_DIR）"
  list_backups
}

cmd_help() {
  cat <<EOF
AIM 生产部署自动化。

用法：$0 <子命令> [参数...]

子命令：
  preflight               校验 /etc/aim、docker、compose、env 必填变量；不修改任何东西
  config-check            preflight 的语义化别名
  init                    第一次部署：把 deploy/{config/prod.example, env/prod.example.env}
                          拷贝到 \$AIM_ENV_FILE 与 \$AIM_CONFIG_DIR。
                          不会覆盖已有内容；如需重建请设置 AIM_INIT_FORCE=1。
  migrate [svc...]        跑 init 服务（默认全跑：auth-migrate logic-migrate
                          kafka-init seaweed-bucket-init）。可指定单个服务。
  up [compose-args...]    流程：preflight -> 备份当前 /etc/aim -> migrate
                                -> docker compose up -d --build [args...]
  down [compose-args...]  流程：备份当前 /etc/aim -> docker compose down [args...]
  restart [compose-args]  等价于 down + up
  status / ps             docker compose ps
  logs [svc] [args...]    docker compose logs 透传
  backup [label]          把 \$AIM_CONFIG_DIR + \$AIM_ENV_FILE 备份到 \$AIM_BACKUP_DIR/<ts>[-label]/
  list-backups            列出所有可用快照
  rollback [id]           从快照恢复 /etc/aim。
                          不指定 id 使用 .latest。
                          必须设置 AIM_ROLLBACK_FORCE=1 才允许覆盖（防误操作）。
  help                    显示本帮助

环境变量：
  AIM_ENV_FILE        默认 /etc/aim/aim.env
  AIM_CONFIG_DIR      默认 /etc/aim/config
  AIM_BACKUP_DIR      默认 /var/backups/aim
  AIM_COMPOSE_DIR     默认 <repo>/deploy/compose
  AIM_INIT_FORCE      init 时设为 1 允许覆盖
  AIM_ROLLBACK_FORCE  rollback 时设为 1 允许覆盖
  AIM_ALLOW_DEV_DEFAULTS  preflight 命中 password=password / access_key=aim 时设为 1 放行

示例：
  sudo $0 init
  sudo $0 preflight
  sudo $0 up
  sudo $0 logs aim-gateway --tail 200
  sudo $0 list-backups
  sudo AIM_ROLLBACK_FORCE=1 $0 rollback
EOF
}

# === 入口 ====================================================================

cmd="${1:-help}"
shift || true

# 需要 root 的子命令提前拦；其余命令继续走。
if need_root "$cmd"; then
  require_root "$cmd"
fi

case "$cmd" in
  preflight)        cmd_preflight "$@" ;;
  init)             cmd_init "$@" ;;
  config-check)     cmd_config_check "$@" ;;
  migrate)          cmd_migrate "$@" ;;
  up)               cmd_up "$@" ;;
  down)             cmd_down "$@" ;;
  restart)          cmd_restart "$@" ;;
  status|ps)        cmd_status "$@" ;;
  logs)             cmd_logs "$@" ;;
  backup)           cmd_backup "$@" ;;
  list-backups)     cmd_list_backups "$@" ;;
  rollback)         cmd_rollback "$@" ;;
  help|-h|--help)   cmd_help ;;
  *)                err "未知子命令: $cmd"; cmd_help; exit 2 ;;
esac
