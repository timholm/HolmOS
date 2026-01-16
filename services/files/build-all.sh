#!/bin/bash

REGISTRY="10.110.67.87:5000"
SERVICES=(
    "file-preview"
    "file-thumbnail"
    "file-compress"
    "file-decompress"
    "file-permissions"
    "file-watch"
    "file-share-create"
    "file-share-validate"
)

for service in "${SERVICES[@]}"; do
    echo "=========================================="
    echo "Building $service..."
    echo "=========================================="

    kubectl run kaniko-${service} \
        --image=gcr.io/kaniko-project/executor:latest \
        --restart=Never \
        --overrides='{
            "spec": {
                "containers": [{
                    "name": "kaniko-'${service}'",
                    "image": "gcr.io/kaniko-project/executor:latest",
                    "args": [
                        "--dockerfile=Dockerfile",
                        "--context=dir:///workspace",
                        "--destination='${REGISTRY}'/holm/'${service}':v1",
                        "--insecure"
                    ],
                    "volumeMounts": [{
                        "name": "build-context",
                        "mountPath": "/workspace"
                    }]
                }],
                "volumes": [{
                    "name": "build-context",
                    "hostPath": {
                        "path": "/tmp/holm-services/files/'${service}'",
                        "type": "Directory"
                    }
                }],
                "restartPolicy": "Never"
            }
        }' \
        -n default

    echo "Waiting for $service build to complete..."
    kubectl wait --for=condition=Ready pod/kaniko-${service} -n default --timeout=300s 2>/dev/null || true
    kubectl wait --for=jsonpath='{.status.phase}'=Succeeded pod/kaniko-${service} -n default --timeout=300s 2>/dev/null || \
    kubectl wait --for=jsonpath='{.status.phase}'=Failed pod/kaniko-${service} -n default --timeout=300s 2>/dev/null || true

    echo "Logs for $service:"
    kubectl logs kaniko-${service} -n default 2>/dev/null || echo "No logs yet"

    # Cleanup
    kubectl delete pod kaniko-${service} -n default --ignore-not-found=true

    echo ""
done

echo "All builds completed!"
