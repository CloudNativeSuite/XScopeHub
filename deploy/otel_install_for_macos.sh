#!/usr/bin/env bash
set -euo pipefail

# ====== 可调参数（通过环境变量覆盖） ======
OO_ORG="${OO_ORG:-default}"                       # OpenObserve org
OO_EMAIL="${OO_EMAIL:-}"                          # 如配置了远端 OO，需提供
OO_PASSWORD="${OO_PASSWORD:-}"                    # 同上
OO_ENDPOINT="${OO_ENDPOINT:-}"                    # 例如 http://oo.example.com:5080/api/${OO_ORG}
HOMEBREW_PREFIX="/opt/homebrew"

OTEL_VERSION="${OTEL_VERSION:-0.133.0}"
OTEL_CONF_DIR="${HOMEBREW_PREFIX}/etc/otelcol-contrib"
OTEL_BIN="${HOMEBREW_PREFIX}/bin/otelcol-contrib"
LAUNCH_PLIST="${HOME}/Library/LaunchAgents/com.observebridge.otelcol.plist"

GRAFANA_ETC="${HOMEBREW_PREFIX}/etc/grafana"
PROV_DATASOURCES="${GRAFANA_ETC}/provisioning/datasources"
PROV_DASHBOARDS="${GRAFANA_ETC}/provisioning/dashboards"
DASHBOARD_DIR="${GRAFANA_ETC}/dashboards"

arch_tag() { [[ "$(uname -m)" == "arm64" ]] && echo "arm64" || echo "amd64"; }
brew_install() { brew list "$1" >/dev/null 2>&1 || brew install "$1"; }
base64_basic() { printf "%s:%s" "$1" "$2" | base64; }

echo "==> 安装并启动 node_exporter"
if ! command -v brew >/dev/null 2>&1; then
  echo "❌ 请先安装 Homebrew: https://brew.sh"
  exit 1
fi
brew_install node_exporter
brew services start node_exporter || true
echo "    - http://localhost:9100/metrics"

echo "⚠️ process-exporter 仅 Linux 可用；macOS 改用 OTel hostmetrics(process/processes)。"

echo "==> 安装并启动 Grafana"
brew_install grafana
brew services start grafana || true
echo "    - http://localhost:3000  (初始 admin/admin)"

# ====== 不使用 Docker：只在提供远端 OO 时启用导出与数据源 ======
USE_OO="0"
if [[ -n "${OO_ENDPOINT}" && -n "${OO_EMAIL}" && -n "${OO_PASSWORD}" ]]; then
  USE_OO="1"
  echo "==> 将使用远端 OpenObserve: ${OO_ENDPOINT} (org=${OO_ORG})"
else
  echo "ℹ️ 未提供 OO_ENDPOINT/OO_EMAIL/OO_PASSWORD，跳过 OpenObserve 集成（可稍后设置环境变量再运行脚本）。"
fi

# ====== 安装 OpenTelemetry Collector（contrib） ======
echo "==> 安装 OpenTelemetry Collector (otelcol-contrib) ${OTEL_VERSION}"
ARCH="$(arch_tag)"
TMPDIR="$(mktemp -d)"
pushd "${TMPDIR}" >/dev/null
TARBALL="otelcol-contrib_${OTEL_VERSION}_darwin_${ARCH}.tar.gz"
URL="https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v${OTEL_VERSION}/${TARBALL}"
curl -fL "$URL" -o "$TARBALL"
tar -xzf "$TARBALL"
sudo mkdir -p "${OTEL_CONF_DIR}"
sudo cp -f otelcol-contrib "${OTEL_BIN}"
sudo chmod +x "${OTEL_BIN}"
popd >/dev/null
rm -rf "${TMPDIR}"

# ====== 生成 OTel 配置 ======
echo "==> 写入 OTel 配置: ${OTEL_CONF_DIR}/config.yaml"
OTEL_EXPORTER_BLOCK=""
if [[ "${USE_OO}" == "1" ]]; then
  OO_AUTH_B64="$(base64_basic "${OO_EMAIL}" "${OO_PASSWORD}")"
  OTEL_EXPORTER_BLOCK="$(cat <<EOF
exporters:
  otlphttp/openobserve:
    endpoint: ${OO_ENDPOINT}
    headers:
      Authorization: "Basic ${OO_AUTH_B64}"
      stream-name: "default"
EOF
)"
  OTEL_PIPELINE_EXPORTERS="[otlphttp/openobserve]"
else
  OTEL_EXPORTER_BLOCK="$(cat <<'EOF'
exporters:
  logging:
    loglevel: info
EOF
)"
  OTEL_PIPELINE_EXPORTERS="[logging]"
fi

sudo tee "${OTEL_CONF_DIR}/config.yaml" >/dev/null <<EOF
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: 'node_exporter'
          static_configs:
            - targets: ['localhost:9100']
  hostmetrics:
    collection_interval: 15s
    scrapers:
      processes: {}
      process: {}

processors:
  batch: {}

${OTEL_EXPORTER_BLOCK}

service:
  telemetry:
    logs:
      level: "info"
  pipelines:
    metrics:
      receivers: [prometheus, hostmetrics]
      processors: [batch]
      exporters: ${OTEL_PIPELINE_EXPORTERS}
EOF

# ====== 配置 launchd 自启动 OTel ======
echo "==> 配置 launchd: ${LAUNCH_PLIST}"
mkdir -p "$(dirname "${LAUNCH_PLIST}")"
cat > "${LAUNCH_PLIST}" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN"
 "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.observebridge.otelcol</string>
  <key>ProgramArguments</key>
  <array>
    <string>${OTEL_BIN}</string>
    <string>--config</string>
    <string>${OTEL_CONF_DIR}/config.yaml</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>${HOME}/Library/Logs/otelcol-contrib.out</string>
  <key>StandardErrorPath</key><string>${HOME}/Library/Logs/otelcol-contrib.err</string>
</dict>
</plist>
PLIST
launchctl unload "${LAUNCH_PLIST}" >/dev/null 2>&1 || true
launchctl load -w "${LAUNCH_PLIST}"

# ====== Grafana 自动配置：仅在 USE_OO=1 时创建 OpenObserve 数据源与仪表盘 ======
if [[ "${USE_OO}" == "1" ]]; then
  echo "==> 配置 Grafana 数据源与仪表盘（OpenObserve Prometheus API）"
  sudo mkdir -p "${PROV_DATASOURCES}" "${PROV_DASHBOARDS}" "${DASHBOARD_DIR}"
  sudo tee "${PROV_DATASOURCES}/openobserve.yaml" >/dev/null <<EOF
apiVersion: 1
datasources:
  - name: OpenObserve
    type: prometheus
    access: proxy
    url: ${OO_ENDPOINT}/prometheus
    isDefault: true
    editable: true
    jsonData:
      httpHeaderName1: "Authorization"
    secureJsonData:
      httpHeaderValue1: "Basic $(printf "%s:%s" "${OO_EMAIL}" "${OO_PASSWORD}" | base64)"
EOF

  sudo tee "${PROV_DASHBOARDS}/openobserve.yaml" >/dev/null <<EOF
apiVersion: 1
providers:
  - name: 'ObserveBridge'
    orgId: 1
    type: file
    folder: ''
    options:
      path: ${DASHBOARD_DIR}
EOF

  sudo tee "${DASHBOARD_DIR}/node_metrics.json" >/dev/null <<'EOF'
{
  "id": null,
  "title": "Node & Process (OTel→OpenObserve)",
  "timezone": "browser",
  "schemaVersion": 38,
  "version": 1,
  "editable": true,
  "panels": [
    {
      "type": "timeseries",
      "title": "CPU Utilization (process)",
      "targets": [{"expr": "process_cpu_utilization", "legendFormat": "{{process}}" }]
    },
    {
      "type": "timeseries",
      "title": "RSS Memory (bytes)",
      "targets": [{"expr": "process_resident_memory_bytes", "legendFormat": "{{process}}" }]
    }
  ]
}
EOF

  echo "    - 已写入数据源与仪表盘；若未显示，可执行：brew services restart grafana"
else
  echo "ℹ️ 跳过 Grafana 数据源与仪表盘（未配置 OpenObserve）。"
  echo "   之后如配置了 OO_* 环境变量，重新运行本脚本即可自动创建。"
fi

echo ""
echo "✅ 完成："
echo "- node_exporter: http://localhost:9100/metrics"
echo "- Grafana:       http://localhost:3000  (首次登录 admin/admin)"
if [[ "${USE_OO}" == "1" ]]; then
  echo "- OpenObserve:   ${OO_ENDPOINT} （已配置 OTel 导出与 Grafana 数据源）"
else
  echo "- OpenObserve:   未配置（当前 OTel 使用 logging 导出）。"
  echo "  若已有远端 OpenObserve：设置如下后重跑脚本："
  echo "    export OO_ORG=default"
  echo "    export OO_ENDPOINT=http://<host>:5080/api/\$OO_ORG"
  echo "    export OO_EMAIL=<email>; export OO_PASSWORD=<password>"
fi
