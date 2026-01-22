helm delete mimir
helm delete otel-collector
helm delete tempo
helm delete grafana
kubectl delete -f ingress.yaml