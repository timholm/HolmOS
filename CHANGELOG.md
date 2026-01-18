# Changelog

All notable changes to HolmOS are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [Unreleased] - 2026-01-17

### Added

#### Kubernetes Infrastructure

- **HorizontalPodAutoscalers (HPA)** for automatic scaling based on CPU utilization
  - `chat-hub-hpa`: Scales chat-hub deployment (1-3 replicas, 70% CPU target)
  - `auth-gateway-hpa`: Scales auth-gateway deployment (1-3 replicas, 70% CPU target)
  - `terminal-web-hpa`: Scales terminal-web deployment (1-3 replicas, 70% CPU target)
  - `file-web-nautilus-hpa`: Scales file-web-nautilus deployment (1-3 replicas, 70% CPU target)
  - Configuration: [k8s/hpa/autoscalers.yaml](k8s/hpa/autoscalers.yaml)

- **PodDisruptionBudgets (PDB)** to ensure high availability during node maintenance
  - `postgres-pdb`: Ensures at least 1 PostgreSQL pod remains available
  - `auth-gateway-pdb`: Ensures at least 1 auth-gateway pod remains available
  - `registry-pdb`: Ensures at least 1 registry pod remains available
  - `holmos-shell-pdb`: Ensures at least 1 holmos-shell pod remains available
  - Configuration: [k8s/pdb/critical-pdbs.yaml](k8s/pdb/critical-pdbs.yaml)

- **NetworkPolicies** for enhanced security and traffic control
  - `allow-holm-internal`: Allows all internal traffic within the holm namespace and permits DNS egress to kube-system
  - Configuration: [k8s/network-policies/policies.yaml](k8s/network-policies/policies.yaml)

- **Secrets Template Manifests** for secure credential management
  - `postgres-secret.yaml`: PostgreSQL database credentials
  - `auth-secrets.yaml`: JWT and admin authentication secrets
  - `ssh-secret.yaml`: SSH credentials for terminal services
  - `backup-storage-db-secret.yaml`: Backup service database credentials
  - `user-preferences-db-secret.yaml`: User preferences database URL
  - Configuration: [k8s/secrets/](k8s/secrets/)

#### Documentation

- **ARCHITECTURE.md**: Comprehensive system architecture documentation
  - Node topology for all 13 Raspberry Pi nodes
  - Service layer diagrams (Application, AI Agents, Core Services, Infrastructure)
  - Data flow diagrams for user requests, AI bot communication, and CI/CD pipeline
  - Storage architecture with PVC details
  - Network architecture with NodePort assignments
  - Security model with RBAC configuration
  - Location: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

- **OPERATIONS.md**: Operations runbook for the cluster
  - Quick reference table for cluster resources
  - Step-by-step guides for deploying and updating services
  - Troubleshooting procedures for common issues
  - Rollback procedures with decision matrix
  - Backup and recovery procedures
  - Monitoring and health check guidance
  - CI/CD pipeline documentation with Kaniko builds
  - Location: [docs/OPERATIONS.md](docs/OPERATIONS.md)

- **SERVICES.md**: Complete service inventory and API documentation
  - Service inventory table with ports, languages, and status
  - Detailed documentation for 30+ services
  - API endpoint documentation for all services
  - Common patterns (health checks, error responses)
  - Environment variables reference
  - Location: [docs/SERVICES.md](docs/SERVICES.md)

- **SECRETS.md**: Secrets management documentation
  - Overview of all secrets used in the cluster
  - Commands for creating and updating secrets
  - Best practices for secrets management
  - Location: [docs/SECRETS.md](docs/SECRETS.md)

### Changed

#### Service Configurations

- **gateway/deployment.yaml**: Added PodDisruptionBudget directly to deployment manifest
  - Ensures gateway service maintains availability during cluster maintenance
  - Location: [services/gateway/deployment.yaml](services/gateway/deployment.yaml)

### Fixed

- N/A

---

## Previous Releases

For changes prior to 2026-01-17, please refer to the git commit history.
