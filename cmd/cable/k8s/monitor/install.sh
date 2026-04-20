#!/bin/bash
set -e

echo "📦 添加 Helm 仓库..."
helm repo add minio https://charts.min.io
helm repo add grafana https://grafana.github.io/helm-charts
# helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
helm repo update

# echo "💾 部署 MinIO..."
# helm upgrade --install minio minio/minio -f 10-minio-values.yaml

# echo "📈 部署 Prometheus..."
# helm upgrade --install prometheus prometheus-community/prometheus -f 20-prometheus-values.yaml

echo "📊 部署 Mimir..."
helm upgrade --install mimir grafana/mimir-distributed -f 10-mimir-values.yaml

echo "🔍 部署 Tempo..."
helm upgrade --install tempo grafana/tempo -f 20-tempo-values.yaml

echo "📡 部署 OpenTelemetry Collector..."
helm upgrade --install otel-collector open-telemetry/opentelemetry-collector -f 30-otel-collector-values.yaml

echo "📊 部署 Grafana..."
helm upgrade --install grafana grafana/grafana -f 40-grafana-values.yaml

echo "🚀 部署 Ingress..."
# kubectl apply -f ingress.yaml

echo "✅ 所有组件已部署！"