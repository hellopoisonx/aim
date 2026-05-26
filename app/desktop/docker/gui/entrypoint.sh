#!/usr/bin/env bash
set -Eeuo pipefail

log() {
  printf '[aim-desktop-gui] %s\n' "$*"
}

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  for pid in "${APP_PID:-}" "${NOVNC_PID:-}" "${VNC_PID:-}" "${OPENBOX_PID:-}" "${XVFB_PID:-}"; do
    if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}" 2>/dev/null || true
    fi
  done
  exit "$status"
}
trap cleanup EXIT INT TERM

if [[ ! -x "${AIM_DESKTOP_BIN}" ]]; then
  log "找不到可执行文件：${AIM_DESKTOP_BIN}"
  log "请先在宿主机执行：cd app/desktop && wails build"
  exit 127
fi

mkdir -p "${AIM_DESKTOP_CONFIG_DIR}"

if [[ ! -f "${AIM_DESKTOP_CONFIG_DIR}/config.json" ]]; then
  python3 - "$AIM_DESKTOP_CONFIG_DIR/config.json" "$AIM_GATEWAY_URL" "$AIM_WS_URL" <<'PY'
import json
import sys

path, gateway_url, ws_url = sys.argv[1:]
with open(path, "w", encoding="utf-8") as f:
    json.dump({"gateway_url": gateway_url, "ws_url": ws_url}, f, ensure_ascii=False, indent=2)
    f.write("\n")
PY
  log "已写入默认配置：${AIM_DESKTOP_CONFIG_DIR}/config.json"
fi

rm -f "/tmp/.X${DISPLAY#:}-lock"
Xvfb "${DISPLAY}" -screen 0 "${SCREEN_GEOMETRY}" -nolisten tcp &
XVFB_PID=$!

for _ in {1..50}; do
  if [[ -S "/tmp/.X11-unix/X${DISPLAY#:}" ]]; then
    break
  fi
  sleep 0.1
done

openbox >/tmp/openbox.log 2>&1 &
OPENBOX_PID=$!

vnc_args=(
  -display "${DISPLAY}"
  -forever
  -shared
  -listen "${VNC_LISTEN}"
  -rfbport "${VNC_PORT}"
  -quiet
)
if [[ -n "${VNC_PASSWORD:-}" ]]; then
  vnc_args+=(-passwd "${VNC_PASSWORD}")
else
  vnc_args+=(-nopw)
  log "VNC 未设置密码；如需密码请设置 VNC_PASSWORD 环境变量。"
fi
x11vnc "${vnc_args[@]}" >/tmp/x11vnc.log 2>&1 &
VNC_PID=$!

/usr/share/novnc/utils/novnc_proxy --listen "${NOVNC_LISTEN}:${NOVNC_PORT}" --vnc "127.0.0.1:${VNC_PORT}" >/tmp/novnc.log 2>&1 &
NOVNC_PID=$!

log "noVNC: http://127.0.0.1:${NOVNC_PORT}/vnc.html"
log "VNC:   127.0.0.1:${VNC_PORT}"
log "Gateway: ${AIM_GATEWAY_URL}"

# WebKitGTK 在部分容器环境会受 sandbox/seccomp 影响。容器默认仅用于本地开发运行 desktop。
export WEBKIT_DISABLE_COMPOSITING_MODE=1
export WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1

# dbus-run-session 让 WebKitGTK / GTK 在容器内拥有完整的 session bus，减少运行时告警。
dbus-run-session -- "${AIM_DESKTOP_BIN}" "$@" &
APP_PID=$!
wait "${APP_PID}"
