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
NEED_ROOT_CMDS=(init up down restart backup rollback config)
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


# === 交互式 config 子命令 ===================================================
# 由 cmd_config 内部复用。整段只依赖 bash + sed + openssl,无第三方依赖。

# 隐藏密码输入;不可恢复时静默降级。
_stty_on()  { stty echo  2>/dev/null || true; }
_stty_off() { stty -echo 2>/dev/null || true; }

gen_secret() {
  # 用 openssl 生成强随机 base64;失败时回退到 /dev/urandom,再不行 sha256sum。
  local bytes="${1:-48}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 "$bytes" | tr -d '\n' && return 0
  fi
  if [[ -r /dev/urandom ]]; then
    head -c "$bytes" /dev/urandom | base64 | tr -d '\n' && return 0
  fi
  # 理论不应触发,但提供最后手段。
  printf '%s%s%s' "$(date +%s%N)" "$RANDOM" "$HOSTNAME" | sha256sum | base64 \
    | head -c "$((bytes * 4 / 3))" | tr -d '\n'
}

prompt_text() {
  # 询问普通文本。$1=提示, $2=默认值(可空), echo 到 stdout。
  # 自动模式($AIM_CONFIG_AUTO=1)下不读取,直接返回默认值或 $3。
  local label="$1" default="${2:-}" fallback="${3:-}"
  if [[ "${AIM_CONFIG_AUTO:-0}" == "1" ]]; then
    printf '%s' "${default:-$fallback}"
    return 0
  fi
  local prompt suffix answer
  if [[ -n "$default" ]]; then
    suffix=" [${default}]"
  else
    suffix=""
  fi
  prompt=$(printf '  %s%s: ' "$label" "$suffix")
  read -r -p "$prompt" answer || true
  if [[ -z "$answer" ]]; then
    answer="$default"
  fi
  printf '%s' "$answer"
}

prompt_secret() {
  # 询问密码/密钥。$1=提示, $2=默认值(可空)。
  # 留空且未提供默认值时,自动生成 48 字节强随机 base64。
  local label="$1" default="${2:-}"
  if [[ "${AIM_CONFIG_AUTO:-0}" == "1" ]]; then
    if [[ -n "$default" ]]; then
      printf '%s' "$default"
    else
      gen_secret 48
    fi
    return 0
  fi
  local prompt answer
  prompt=$(printf '  %s [回车自动生成, 已有值回车保留]: ' "$label")
  _stty_off
  read -r -p "$prompt" answer || true
  _stty_on
  # 装饰性换行走 stderr,避免被 $(prompt_secret) 一起捕获。
  printf '\n' >&2
  if [[ -n "$answer" ]]; then
    printf '%s' "$answer"
  elif [[ -n "$default" ]]; then
    printf '%s' "$default"
  else
    gen_secret 48
  fi
}

prompt_confirm() {
  # y/N 确认。$1=提示, $2=默认 y/n(1=y, 0=n)。
  local label="$1" default_yes="${2:-1}"
  if [[ "${AIM_CONFIG_AUTO:-0}" == "1" ]]; then
    [[ "$default_yes" == "1" ]] && return 0 || return 1
  fi
  local suffix prompt answer
  if [[ "$default_yes" == "1" ]]; then
    suffix=" [Y/n]"
  else
    suffix=" [y/N]"
  fi
  prompt=$(printf '  %s%s: ' "$label" "$suffix")
  read -r -p "$prompt" answer || true
  case "${answer,,}" in
    y|yes) return 0 ;;
    n|no)  return 1 ;;
    *)     [[ "$default_yes" == "1" ]] && return 0 || return 1 ;;
  esac
}

env_get_value() {
  # 从 env 文件读 key 的 value。失败返回 1,空 value 视为成功。
  # CHANGE_ME_* 占位符视为未设置,避免被当作“已有值”传给 prompt_* 而跳
  # 过自动生成。
  local file="$1" key="$2"
  [[ -f "$file" ]] || return 1
  local line value
  line=$(grep -E "^[[:space:]]*${key}=" "$file" 2>/dev/null | head -n1 || true)
  [[ -n "$line" ]] || return 1
  value="${line#*=}"
  [[ "$value" =~ ^CHANGE_ME_[A-Z0-9_]*$ ]] && return 1
  printf '%s' "$value"
}

env_set_value() {
  # 设置 env 文件中的 key=value。value 可能是 base64(含 / + =),用 | 作 sed 分隔符。
  local file="$1" key="$2" value="$3"
  local esc
  esc=$(printf '%s' "$value" | sed 's/[\\&|]/\\&/g')
  if grep -qE "^[[:space:]]*${key}=" "$file" 2>/dev/null; then
    sed -i "s|^[[:space:]]*${key}=.*|${key}=${esc}|" "$file"
  else
    printf '%s=%s\n' "$key" "$value" >> "$file"
  fi
}

env_unset_key() {
  # 从 env 文件移除指定 key(注释行不动,仅删除未注释的赋值行)。
  local file="$1" key="$2"
  [[ -f "$file" ]] || return 0
  sed -i.bak -E "/^[[:space:]]*${key}=/d" "$file" && rm -f "${file}.bak"
}

replace_placeholder_in_dir() {
  # 在 config 目录下所有 yaml/json 中替换占位符。占位符形式: 整词 CHANGE_ME_XXX。
  local placeholder="$1" value="$2"
  local esc f
  esc=$(printf '%s' "$value" | sed 's/[\\&|]/\\&/g')
  for f in "$AIM_CONFIG_DIR"/*.yaml "$AIM_CONFIG_DIR"/*.json; do
    [[ -f "$f" ]] || continue
    sed -i "s|${placeholder}|${esc}|g" "$f"
  done
}

yaml_delete_field() {
  # 从 yaml 中删除指定顶层字段行(每行格式: "  <key>: <value>",允许缩进变化)。
  local key="$1"
  local f
  for f in "$AIM_CONFIG_DIR"/*.yaml; do
    [[ -f "$f" ]] || continue
    sed -i.bak -E "/^[[:space:]]*${key}:[[:space:]].*/d" "$f" && rm -f "${f}.bak"
  done
}

yaml_set_field_in_dir() {
  # 强制覆盖 config 目录下所有 yaml/json 中同名字段的值。
  # 既能处理首次的 CHANGE_ME_* 占位符(此时 ask_* 会同时调 replace_placeholder_in_dir)
  # 也能在重跑 config 时覆盖已被手动改过的同名字段。
  # 字段名需在 yaml/json 中完全一致(JSON 可能是 camelCase)。
  local key="$1" value="$2"
  local esc f
  esc=$(printf '%s' "$value" | sed 's/[\\&|]/\\&/g')
  for f in "$AIM_CONFIG_DIR"/*.yaml; do
    [[ -f "$f" ]] || continue
    sed -i -E "s|(^[[:space:]]*${key}:[[:space:]]+).*|\1${esc}|" "$f"
  done
  for f in "$AIM_CONFIG_DIR"/*.json; do
    [[ -f "$f" ]] || continue
    sed -i -E "s|(\"${key}\"[[:space:]]*:[[:space:]]*\")[^\"]*|\1${esc}|" "$f"
  done
}

yaml_set_dsn_password_in_dir() {
  # 强制更新 config 目录下所有 yaml 中 postgres://user:PWD@... 的密码。
  # 跨 attachment.yaml / data_parsing.yaml / auth.yaml / logic.yaml / core.yaml。
  local new_pw="$1"
  local esc f
  esc=$(printf '%s' "$new_pw" | sed 's/[\\&|]/\\&/g')
  for f in "$AIM_CONFIG_DIR"/*.yaml; do
    [[ -f "$f" ]] || continue
    sed -i -E "s|(DataSource:[[:space:]]*postgres://[^:]+:)[^@]+(@)|\1${esc}\2|" "$f"
  done
}

yaml_set_etcd_hosts_in_dir() {
  # 强制更新 config 目录下所有 yaml 中 etcd Hosts 列表。
  # 使用 awk 一次扫描、按 1/2/3 循环编号，避免 sed 多步替换的竞态。
  # 仅匹配 "- host:2379" 行（不限制 host 字符集）。
  local h1="$1" h2="$2" h3="$3" f
  for f in "$AIM_CONFIG_DIR"/*.yaml; do
    [[ -f "$f" ]] || continue
    awk -v H1="$h1" -v H2="$h2" -v H3="$h3" '
      /^[[:space:]]+- [A-Za-z0-9._:-]+:2379[[:space:]]*$/ {
        n = (++count % 3); if (n == 0) n = 3
        if (n == 1) sub(/- [A-Za-z0-9._:-]+:2379/, "- " H1 ":2379")
        else if (n == 2) sub(/- [A-Za-z0-9._:-]+:2379/, "- " H2 ":2379")
        else sub(/- [A-Za-z0-9._:-]+:2379/, "- " H3 ":2379")
      }
      { print }
    ' "$f" > "$f.tmp" && mv "$f.tmp" "$f"
  done
}

# --- 各 group 询问 ---------------------------------------------------------

ask_postgres() {
  section "[1/8] PostgreSQL"
  local user pw db
  user=$(prompt_text  "POSTGRES_USER"     "$(env_get_value "$AIM_ENV_FILE" POSTGRES_USER || echo aim_user)")
  pw=$(prompt_secret   "POSTGRES_PASSWORD" "$(env_get_value "$AIM_ENV_FILE" POSTGRES_PASSWORD || true)")
  db=$(prompt_text    "POSTGRES_DB"        "$(env_get_value "$AIM_ENV_FILE" POSTGRES_DB || echo postgres)")
  env_set_value "$AIM_ENV_FILE" POSTGRES_USER     "$user"
  env_set_value "$AIM_ENV_FILE" POSTGRES_PASSWORD "$pw"
  env_set_value "$AIM_ENV_FILE" POSTGRES_DB       "$db"
  # 同步到 4 处 DSN (auth/logic/core/data_parsing 各自的 DataSource)。
  # 首次走占位符替换,重跑走 DSN 密码强制更新。
  replace_placeholder_in_dir "CHANGE_ME_POSTGRES_PASSWORD" "$pw"
  yaml_set_dsn_password_in_dir "$pw"
  ok "PostgreSQL 已写入 env + 4 个 yaml DSN"
}

ask_gateway_node_id() {
  section "[2/8] AIM 节点"
  local id
  id=$(prompt_text "AIM_GATEWAY_NODE_ID (Snowflake / 0-1023)" \
                   "$(env_get_value "$AIM_ENV_FILE" AIM_GATEWAY_NODE_ID || echo 0)")
  env_set_value "$AIM_ENV_FILE" AIM_GATEWAY_NODE_ID "$id"
}

ask_etcd_hosts() {
  section "[3/8] etcd 集群地址"
  local cur1 cur2 cur3 h1 h2 h3
  # set -e + pipefail: 拿不到已有值时 grep 返回 1, 用 || true 避免亚 shell 拋出退出码。
  cur1=$( ( grep -hE 'CHANGE_ME_ETCD_HOST_1' "$AIM_CONFIG_DIR"/*.yaml 2>/dev/null || true ) | head -n1 \
          | sed -E 's/.*- ([^:]+):.*/\1/' || true)
  cur2=$( ( grep -hE 'CHANGE_ME_ETCD_HOST_2' "$AIM_CONFIG_DIR"/*.yaml 2>/dev/null || true ) | head -n1 \
          | sed -E 's/.*- ([^:]+):.*/\1/' || true)
  cur3=$( ( grep -hE 'CHANGE_ME_ETCD_HOST_3' "$AIM_CONFIG_DIR"/*.yaml 2>/dev/null || true ) | head -n1 \
          | sed -E 's/.*- ([^:]+):.*/\1/' || true)
  [[ -z "$cur1" || "$cur1" =~ CHANGE_ME_ETCD_HOST_1 ]] && cur1="etcd1"
  [[ -z "$cur2" || "$cur2" =~ CHANGE_ME_ETCD_HOST_2 ]] && cur2="etcd2"
  [[ -z "$cur3" || "$cur3" =~ CHANGE_ME_ETCD_HOST_3 ]] && cur3="etcd3"
  h1=$(prompt_text "etcd node 1 (host or IP)"        "$cur1")
  h2=$(prompt_text "etcd node 2 (host or IP)"        "$cur2")
  h3=$(prompt_text "etcd node 3 (host or IP, 单节点可与 node1 相同)" "$cur3")
  # 首次走占位符替换,重跑走 hosts 列表强制更新。
  replace_placeholder_in_dir "CHANGE_ME_ETCD_HOST_1:2379" "${h1}:2379"
  replace_placeholder_in_dir "CHANGE_ME_ETCD_HOST_2:2379" "${h2}:2379"
  replace_placeholder_in_dir "CHANGE_ME_ETCD_HOST_3:2379" "${h3}:2379"
  yaml_set_etcd_hosts_in_dir "$h1" "$h2" "$h3"
  ok "etcd hosts 已同步至所有 yaml"
}

ask_etcd_auth() {
  section "[4/8] etcd 鉴权 / TLS"
  if prompt_confirm "启用 etcd 鉴权 (User/Pass + TLS 证书)? (Y/n)"; then
    local u p cert key ca
    u=$(prompt_text   "ETCD_USERNAME"   "$(env_get_value "$AIM_ENV_FILE" ETCD_USERNAME || echo root)")
    p=$(prompt_secret   "ETCD_PASSWORD"   "$(env_get_value "$AIM_ENV_FILE" ETCD_PASSWORD || true)")
    cert=$(prompt_text  "ETCD_CERT_FILE"  "$(env_get_value "$AIM_ENV_FILE" ETCD_CERT_FILE || echo /etc/etcd/certs/etcd.pem)")
    key=$(prompt_text   "ETCD_KEY_FILE"   "$(env_get_value "$AIM_ENV_FILE" ETCD_KEY_FILE || echo /etc/etcd/certs/etcd-key.pem)")
    ca=$(prompt_text    "ETCD_CA_FILE"    "$(env_get_value "$AIM_ENV_FILE" ETCD_CA_FILE || echo /etc/etcd/certs/ca.pem)")
    env_set_value "$AIM_ENV_FILE" ETCD_USERNAME   "$u"
    env_set_value "$AIM_ENV_FILE" ETCD_PASSWORD   "$p"
    env_set_value "$AIM_ENV_FILE" ETCD_CERT_FILE  "$cert"
    env_set_value "$AIM_ENV_FILE" ETCD_KEY_FILE   "$key"
    env_set_value "$AIM_ENV_FILE" ETCD_CA_FILE    "$ca"
    # 首次走占位符,重跑走强制覆盖(从关闭鉴权后重开场景下仍能恢复 yaml 字段)。
    replace_placeholder_in_dir "CHANGE_ME_ETCD_USER"       "$u"
    replace_placeholder_in_dir "CHANGE_ME_ETCD_PASSWORD"   "$p"
    replace_placeholder_in_dir "CHANGE_ME_ETCD_CERT_FILE"  "$cert"
    replace_placeholder_in_dir "CHANGE_ME_ETCD_KEY_FILE"   "$key"
    replace_placeholder_in_dir "CHANGE_ME_ETCD_CA_FILE"    "$ca"
    yaml_set_field_in_dir User     "$u"
    yaml_set_field_in_dir Pass     "$p"
    yaml_set_field_in_dir CertFile "$cert"
    yaml_set_field_in_dir CertKeyFile "$key"
    yaml_set_field_in_dir CACertFile  "$ca"
    ok "etcd 鉴权/TLS 已写入 env 与所有 yaml"
  else
    # 关闭鉴权:从 env 与所有 yaml 中删除相关 key/字段
    env_unset_key "$AIM_ENV_FILE" ETCD_USERNAME
    env_unset_key "$AIM_ENV_FILE" ETCD_PASSWORD
    env_unset_key "$AIM_ENV_FILE" ETCD_CERT_FILE
    env_unset_key "$AIM_ENV_FILE" ETCD_KEY_FILE
    env_unset_key "$AIM_ENV_FILE" ETCD_CA_FILE
    yaml_delete_field User
    yaml_delete_field Pass
    yaml_delete_field CertFile
    yaml_delete_field CertKeyFile
    yaml_delete_field CACertFile
    ok "etcd 鉴权/TLS 已关闭 (env 字段与 yaml 字段已移除)"
  fi
}

ask_seaweed() {
  section "[5/8] SeaweedFS (S3 兼容存储)"
  local ak sk bucket region endpoint
  ak=$(prompt_text   "SEAWEED_ACCESS_KEY"   "$(env_get_value "$AIM_ENV_FILE" SEAWEED_ACCESS_KEY || echo aim-prod)")
  sk=$(prompt_secret   "SEAWEED_SECRET_KEY"   "$(env_get_value "$AIM_ENV_FILE" SEAWEED_SECRET_KEY || true)")
  bucket=$(prompt_text "SEAWEED_BUCKET"      "$(env_get_value "$AIM_ENV_FILE" SEAWEED_BUCKET || echo aim-attachments)")
  region=$(prompt_text "SEAWEED_REGION"      "$(env_get_value "$AIM_ENV_FILE" SEAWEED_REGION || echo us-east-1)")
  endpoint=$(prompt_text "SEAWEED_S3_ENDPOINT (容器内 S3 端点)" \
                            "$(env_get_value "$AIM_ENV_FILE" SEAWEED_S3_ENDPOINT || echo http://seaweed-s3:8333)")
  env_set_value "$AIM_ENV_FILE" SEAWEED_ACCESS_KEY  "$ak"
  env_set_value "$AIM_ENV_FILE" SEAWEED_SECRET_KEY  "$sk"
  env_set_value "$AIM_ENV_FILE" SEAWEED_BUCKET      "$bucket"
  env_set_value "$AIM_ENV_FILE" SEAWEED_REGION      "$region"
  env_set_value "$AIM_ENV_FILE" SEAWEED_S3_ENDPOINT "$endpoint"
  # 首次走占位符,重跑走强制覆盖(yaml 字段名:SecretKey;JSON 字段名:secretKey)。
  replace_placeholder_in_dir "CHANGE_ME_SEAWEED_SECRET_KEY" "$sk"
  yaml_set_field_in_dir SecretKey "$sk"
  yaml_set_field_in_dir secretKey "$sk"
  ok "SeaweedFS 已写入 env + attachment.yaml + data_parsing.yaml + seaweed-s3.json"
}

ask_files_domain() {
  section "[6/8] 附件外部域名"
  local cur domain
  # set -e + pipefail: grep 未命中返回 1,用 || true 避免亚 shell 拋出退出码。
  cur=$( ( grep -hE 'CHANGE_ME_FILES_DOMAIN' "$AIM_CONFIG_DIR"/*.yaml 2>/dev/null || true ) | head -n1 \
        | sed -E 's/.*https:\/\///' || true)
  [[ -z "$cur" || "$cur" =~ CHANGE_ME ]] && cur="files.example.com"
  domain=$(prompt_text "PublicEndpoint (客户端访问附件的域名, 例如 files.your-domain.com)" "$cur")
  # 首次走占位符,重跑走强制覆盖。
  replace_placeholder_in_dir "CHANGE_ME_FILES_DOMAIN" "$domain"
  yaml_set_field_in_dir PublicEndpoint "$domain"
  ok "附件域名已同步至 attachment.yaml"
}

ask_grafana() {
  section "[7/8] Grafana"
  local u p
  u=$(prompt_text  "GF_SECURITY_ADMIN_USER"     "$(env_get_value "$AIM_ENV_FILE" GF_SECURITY_ADMIN_USER || echo admin)")
  p=$(prompt_secret "GF_SECURITY_ADMIN_PASSWORD" "$(env_get_value "$AIM_ENV_FILE" GF_SECURITY_ADMIN_PASSWORD || true)")
  env_set_value "$AIM_ENV_FILE" GF_SECURITY_ADMIN_USER     "$u"
  env_set_value "$AIM_ENV_FILE" GF_SECURITY_ADMIN_PASSWORD "$p"
  replace_placeholder_in_dir "CHANGE_ME_GRAFANA_ADMIN_PASSWORD" "$p"
  ok "Grafana 凭据已写入 env"
}

ask_jwt() {
  section "[8/8] JWT AccessSecret (auth + gateway-api)"
  local cur secret
  # set -e + pipefail: grep 未命中返回 1,用 || true 避免亚 shell 拋出退出码。
  cur=$( ( grep -hE 'CHANGE_ME_RANDOM_48_BYTES' "$AIM_CONFIG_DIR"/*.yaml 2>/dev/null || true ) | head -n1 \
        | sed -E 's/.*: //' || true)
  [[ -z "$cur" || "$cur" =~ CHANGE_ME ]] && cur=""
  secret=$(prompt_secret "JWT AccessSecret" "$cur")
  # 首次走占位符,重跑走强制覆盖。
  replace_placeholder_in_dir "CHANGE_ME_RANDOM_48_BYTES" "$secret"
  yaml_set_field_in_dir AccessSecret "$secret"
  ok "JWT AccessSecret 已同步至 auth.yaml + gateway-api.yaml"
}

# --- 主流程 ----------------------------------------------------------------

cmd_config() {
  require_root "$@"
  section "config: 交互式填写 $AIM_ENV_FILE 与 $AIM_CONFIG_DIR"
  require_repo_layout

  # 解析 --auto / --no-backup
  local auto=0 no_backup=0
  local arg
  for arg in "$@"; do
    case "$arg" in
      --auto)      auto=1 ;;
      --no-backup) no_backup=1 ;;
      *)           die "未知参数: $arg (支持 --auto / --no-backup)" ;;
    esac
  done
  if [[ "$auto" == "1" ]]; then
    export AIM_CONFIG_AUTO=1
    log "AIM_CONFIG_AUTO=1: 所有密钥自动生成, 域名类字段仍会询问"
  fi

  # 1) 确认 env/config 已 init
  if [[ ! -f "$AIM_ENV_FILE" || ! -d "$AIM_CONFIG_DIR" ]]; then
    warn "$AIM_ENV_FILE 或 $AIM_CONFIG_DIR 不存在"
    if prompt_confirm "现在自动执行 init? (Y/n)"; then
      cmd_init
    else
      die "请先运行 '$0 init'。"
    fi
  fi
  require_env_file
  require_config_dir

  # 2) 备份当前 (除非 --no-backup)
  if [[ "$no_backup" == "0" ]]; then
    run_backup "before-config" || warn "备份失败,继续..."
  fi

  # 3) 逐组询问
  ask_postgres
  ask_gateway_node_id
  ask_etcd_hosts
  ask_etcd_auth
  ask_seaweed
  ask_files_domain
  ask_grafana
  ask_jwt

  # 4) 收尾
  section "config: 完成"
  ok "所有配置已写回 $AIM_ENV_FILE 与 $AIM_CONFIG_DIR"
  log "运行 '$0 preflight' 校验..."
  cmd_preflight
  cat <<'NEXT'

下一步:
  1) 若启用了 etcd 鉴权/TLS, 把证书放到对应 CertFile 路径并保证容器可访问
  2) sudo deploy/deploy.sh up     # 启动生产栈
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
  config [--auto]         交互式填写 \$AIM_ENV_FILE 与 \$AIM_CONFIG_DIR 中所有 CHANGE_ME。
                          8 个分组：PostgreSQL / 节点 / etcd 集群 / etcd 鉴权 / SeaweedFS /
                          附件域名 / Grafana / JWT。密码类字段回车自动生成。
                          --auto: 跳过询问(密钥自动生成, 域名仍需填写)。
                          --no-backup: 不创建 before-config 快照。
                          会先备份 /etc/aim，再写入，最后跑 preflight。
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
  sudo $0 config            # 交互式填写 /etc/aim 中的 CHANGE_ME
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
  config)           cmd_config "$@" ;;
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
