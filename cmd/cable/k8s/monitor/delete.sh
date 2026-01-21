helm delete minio -n observability 
helm delete tempo -n observability 
helm delete prometheus -n observability 
helm delete grafana -n observability 
helm delete otel-collector -n observability 