#!/bin/bash
# Tekton Build Trigger for AI Bots
# Usage: ./tekton-build.sh [tag]
#   tag: Optional image tag (default: v$(date +%Y%m%d%H%M))

TAG="${1:-v$(date +%Y%m%d%H%M)}"
RUN_NAME="build-ai-bots-$(date +%s)"

echo "🚀 Triggering Tekton build for ai-bots:${TAG}"

# Create PipelineRun
kubectl create -f - <<EOF
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: ${RUN_NAME}
  namespace: holm
spec:
  pipelineRef:
    name: build-ai-bots
  params:
    - name: image-tag
      value: "${TAG}"
  workspaces:
    - name: shared-workspace
      persistentVolumeClaim:
        claimName: tekton-workspace-pvc
EOF

echo "📋 Pipeline run created: ${RUN_NAME}"
echo ""
echo "Watch progress with:"
echo "  kubectl get pipelineruns -n holm -w"
echo "  kubectl logs -n holm -l tekton.dev/pipelineRun=${RUN_NAME} -f"
