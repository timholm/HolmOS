# HolmOS Makefile
# Simple commands for managing the Pi cluster

PI_HOST := 192.168.8.197
PI_USER := rpi1
REGISTRY := 10.110.67.87:5000

.PHONY: help status deploy build ssh sync push pull health logs test smoke

help:
	@echo "HolmOS Commands:"
	@echo "  make status     - Show cluster status"
	@echo "  make health     - Check all services health"
	@echo "  make ssh        - SSH to control plane"
	@echo "  make sync       - Sync services from cluster"
	@echo "  make push       - Push changes to GitHub"
	@echo "  make deploy S=  - Deploy specific service"
	@echo "  make build S=   - Build specific service"
	@echo "  make logs S=    - View logs for service"
	@echo "  make restart S= - Restart service"
	@echo "  make shell S=   - Exec into service pod"
	@echo ""
	@echo "Testing Commands:"
	@echo "  make test       - Run all tests"
	@echo "  make smoke      - Run smoke tests (3 core tests)"
	@echo "  make smoke-quick    - Quick single service check"
	@echo "  make smoke-degraded - Allow partial failures"

status:
	@sshpass -p "$(PI_PASS)" ssh -o StrictHostKeyChecking=no $(PI_USER)@$(PI_HOST) \
		"kubectl get nodes; echo '---'; kubectl get pods -n holm | head -20"

health:
	@sshpass -p "$(PI_PASS)" ssh -o StrictHostKeyChecking=no $(PI_USER)@$(PI_HOST) \
		"kubectl get pods -n holm --no-headers | awk '{print \$$1, \$$3}' | column -t"

ssh:
	@sshpass -p "$(PI_PASS)" ssh -o StrictHostKeyChecking=no $(PI_USER)@$(PI_HOST)

sync:
	@echo "Syncing services from cluster..."
	@sshpass -p "$(PI_PASS)" ssh -o StrictHostKeyChecking=no $(PI_USER)@$(PI_HOST) \
		"cd /home/rpi1 && tar czf /tmp/builds.tar.gz builds"
	@sshpass -p "$(PI_PASS)" scp -o StrictHostKeyChecking=no \
		$(PI_USER)@$(PI_HOST):/tmp/builds.tar.gz /tmp/
	@tar xzf /tmp/builds.tar.gz -C /tmp/
	@rsync -av /tmp/builds/ services/
	@rm -rf /tmp/builds /tmp/builds.tar.gz
	@echo "Sync complete!"

push:
	@git add -A && git commit -m "Update services" && git push

deploy:
ifndef S
	$(error S is not set. Usage: make deploy S=holmos-shell)
endif
	@echo "Deploying $(S)..."
	@gh workflow run deploy.yml -f service=$(S)

build:
ifndef S
	$(error S is not set. Usage: make build S=holmos-shell)
endif
	@echo "Building $(S)..."
	@cd services/$(S) && docker buildx build --platform linux/arm64 -t $(REGISTRY)/$(S):latest .

logs:
ifndef S
	$(error S is not set. Usage: make logs S=holmos-shell)
endif
	@sshpass -p "$(PI_PASS)" ssh -o StrictHostKeyChecking=no $(PI_USER)@$(PI_HOST) \
		"kubectl logs -n holm -l app=$(S) --tail=100 -f"

restart:
ifndef S
	$(error S is not set. Usage: make restart S=holmos-shell)
endif
	@sshpass -p "$(PI_PASS)" ssh -o StrictHostKeyChecking=no $(PI_USER)@$(PI_HOST) \
		"kubectl rollout restart deployment/$(S) -n holm"

shell:
ifndef S
	$(error S is not set. Usage: make shell S=holmos-shell)
endif
	@sshpass -p "$(PI_PASS)" ssh -o StrictHostKeyChecking=no $(PI_USER)@$(PI_HOST) \
		"kubectl exec -it -n holm \$$(kubectl get pods -n holm -l app=$(S) -o jsonpath='{.items[0].metadata.name}') -- /bin/sh"

# Quick commands
pods:
	@sshpass -p "$(PI_PASS)" ssh -o StrictHostKeyChecking=no $(PI_USER)@$(PI_HOST) \
		"kubectl get pods -n holm"

services:
	@sshpass -p "$(PI_PASS)" ssh -o StrictHostKeyChecking=no $(PI_USER)@$(PI_HOST) \
		"kubectl get svc -n holm -o custom-columns=NAME:.metadata.name,PORT:.spec.ports[0].nodePort"

images:
	@curl -s http://$(REGISTRY)/v2/_catalog | jq -r '.repositories[]' | sort

nodes:
	@sshpass -p "$(PI_PASS)" ssh -o StrictHostKeyChecking=no $(PI_USER)@$(PI_HOST) \
		"kubectl get nodes -o wide"

top:
	@sshpass -p "$(PI_PASS)" ssh -o StrictHostKeyChecking=no $(PI_USER)@$(PI_HOST) \
		"kubectl top nodes; echo '---'; kubectl top pods -n holm | head -20"

# Testing commands
test:
	@cd tests && npm test

smoke:
	@cd tests && npm run test:smoke

smoke-quick:
	@cd tests && npm run test:smoke:quick

smoke-degraded:
	@cd tests && npm run test:smoke:degraded
