# HolmOS Ingress Controller Plan

**Created:** 2026-01-17
**Status:** Planning
**Author:** Infrastructure Team

---

## Executive Summary

This document outlines the strategy for implementing a robust ingress controller system for HolmOS external access. The plan leverages the existing Traefik installation (k3s default) and provides a path to production-ready external access with SSL/TLS, rate limiting, and security hardening.

---

## 1. Current State Analysis

### 1.1 Existing Ingress Infrastructure

**Ingress Controller:** Traefik v3.5.1 (k3s default)
- **Status:** Running and healthy
- **Namespace:** kube-system
- **IngressClass:** `traefik` (available)
- **Dashboard:** Enabled (port 8080)
- **Metrics:** Prometheus-enabled (port 9100)

**Load Balancer IPs (via k3s ServiceLB):**
```
192.168.8.105, 192.168.8.108, 192.168.8.187, 192.168.8.194,
192.168.8.195, 192.168.8.196, 192.168.8.197, 192.168.8.199,
192.168.8.202, 192.168.8.209, 192.168.8.210, 192.168.8.231, 192.168.8.235
```

**Traefik Service Ports:**
- HTTP: 80 (NodePort 30190)
- HTTPS: 443 (NodePort 32762)

**Existing Ingress Resources:**
| Namespace | Name | Class | Hosts | Status |
|-----------|------|-------|-------|--------|
| holm | auth-gateway | nginx | auth.holm.local | Misconfigured (wrong class) |

### 1.2 Current NodePort Services

All services are currently exposed via NodePort, which is not ideal for production:

| Service | Port | NodePort | Purpose |
|---------|------|----------|---------|
| holmos-shell | 80 | 30000 | Main HolmOS shell interface |
| ios-shell | 80 | 30001 | iOS-optimized shell |
| app-store-ai | 80 | 30002 | App store interface |
| chat-hub | 80 | 30003 | Chat application hub |
| nova | 80 | 30004 | Nova AI interface |
| pulse | 8080 | 30006 | Monitoring service |
| holm-git | 8080 | 30009 | Git server interface |
| calculator-app | 80 | 30010 | Calculator application |
| clock-app | 80 | 30011 | Clock application |
| deploy-controller | 80 | 30015 | Deployment controller |
| backup-svc | 80 | 30016 | Backup service |
| scribe | 80 | 30017 | Documentation service |
| cicd-controller | 8080 | 30020 | CI/CD controller |
| file-web-nautilus | 80 | 30088 | File manager |
| steve-bot | 8080 | 30099 | Steve bot interface |
| auth-gateway-nodeport | 80 | 30100 | Authentication gateway |
| cluster-manager | 80 | 30502 | Cluster management |
| settings-web | 8080 | 30600 | Settings interface |
| alice-bot | 8080 | 30668 | Alice bot interface |
| health-agg | 8080 | 30667 | Health aggregator |
| audiobook-web-nodeport | 8080 | 30700 | Audiobook player |
| terminal-web | 80 | 30800 | Web terminal |
| vault | 80 | 30870 | Secrets management |
| metrics-dashboard | 8080 | 30950 | Metrics dashboard |
| registry | 5000 | 31500 | Container registry |
| registry-ui | 8080 | 31750 | Registry UI |

**Registry Namespace:**
| Service | Port | NodePort | Purpose |
|---------|------|----------|---------|
| registry | 5000 | 30500 | Additional registry endpoint |

---

## 2. Ingress Controller Recommendation

### 2.1 Decision: Use Traefik (Already Installed)

**Rationale:**
1. Already installed and running as k3s default
2. Native Kubernetes Ingress support + powerful CRD extensions
3. Built-in Let's Encrypt integration (ACME)
4. Dashboard for monitoring and debugging
5. Excellent middleware support for security features
6. Lower resource footprint than nginx-ingress
7. Active development and k3s-optimized

### 2.2 Alternative: nginx-ingress

If Traefik proves insufficient, nginx-ingress can be installed alongside:
```bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm install nginx-ingress ingress-nginx/ingress-nginx \
  --namespace ingress-nginx --create-namespace \
  --set controller.ingressClassResource.name=nginx \
  --set controller.ingressClassResource.default=false
```

---

## 3. SSL/TLS Strategy

### 3.1 Certificate Management with cert-manager

**Installation:**
```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml

# Wait for deployment
kubectl wait --for=condition=available --timeout=300s deployment/cert-manager -n cert-manager
kubectl wait --for=condition=available --timeout=300s deployment/cert-manager-webhook -n cert-manager
```

### 3.2 Let's Encrypt Issuers

**Staging Issuer (for testing):**
```yaml
# cert-manager/staging-issuer.yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
spec:
  acme:
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    email: admin@holmos.local  # Replace with real email
    privateKeySecretRef:
      name: letsencrypt-staging-key
    solvers:
    - http01:
        ingress:
          class: traefik
```

**Production Issuer:**
```yaml
# cert-manager/production-issuer.yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-production
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@holmos.local  # Replace with real email
    privateKeySecretRef:
      name: letsencrypt-production-key
    solvers:
    - http01:
        ingress:
          class: traefik
```

### 3.3 DNS Challenge (for Wildcard Certificates)

For wildcard certificates (`*.holmos.example.com`), use DNS-01 challenge:

```yaml
# cert-manager/dns-issuer.yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-dns
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@holmos.local
    privateKeySecretRef:
      name: letsencrypt-dns-key
    solvers:
    - dns01:
        cloudflare:  # Or your DNS provider
          email: admin@holmos.local
          apiTokenSecretRef:
            name: cloudflare-api-token
            key: api-token
```

### 3.4 Self-Signed Certificates (Internal/Development)

```yaml
# cert-manager/selfsigned-issuer.yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: holmos-ca
  namespace: cert-manager
spec:
  isCA: true
  commonName: holmos-ca
  secretName: holmos-ca-secret
  privateKey:
    algorithm: ECDSA
    size: 256
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
---
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: holmos-ca-issuer
spec:
  ca:
    secretName: holmos-ca-secret
```

---

## 4. Sample Ingress Resources

### 4.1 Main Shell Interface (Traefik IngressRoute)

```yaml
# ingress/holmos-shell-ingress.yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: holmos-shell
  namespace: holm
spec:
  entryPoints:
    - websecure
  routes:
    - match: Host(`holmos.example.com`) || Host(`shell.holmos.example.com`)
      kind: Rule
      services:
        - name: holmos-shell
          port: 80
      middlewares:
        - name: security-headers
        - name: rate-limit
  tls:
    certResolver: letsencrypt  # If using Traefik ACME
    # OR use cert-manager:
    # secretName: holmos-shell-tls
```

### 4.2 Standard Kubernetes Ingress (Alternative)

```yaml
# ingress/holmos-shell-k8s-ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: holmos-shell
  namespace: holm
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-production
    traefik.ingress.kubernetes.io/router.middlewares: holm-security-headers@kubernetescrd,holm-rate-limit@kubernetescrd
spec:
  ingressClassName: traefik
  tls:
    - hosts:
        - holmos.example.com
        - shell.holmos.example.com
      secretName: holmos-shell-tls
  rules:
    - host: holmos.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: holmos-shell
                port:
                  number: 80
    - host: shell.holmos.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: holmos-shell
                port:
                  number: 80
```

### 4.3 Nova AI Interface

```yaml
# ingress/nova-ingress.yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: nova
  namespace: holm
spec:
  entryPoints:
    - websecure
  routes:
    - match: Host(`nova.holmos.example.com`)
      kind: Rule
      services:
        - name: nova
          port: 80
      middlewares:
        - name: security-headers
        - name: rate-limit
        - name: auth-forward  # Optional: Forward auth to auth-gateway
  tls:
    secretName: nova-tls
```

### 4.4 Auth Gateway

```yaml
# ingress/auth-gateway-ingress.yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: auth-gateway
  namespace: holm
spec:
  entryPoints:
    - websecure
  routes:
    - match: Host(`auth.holmos.example.com`)
      kind: Rule
      services:
        - name: auth-gateway-nodeport
          port: 80
      middlewares:
        - name: security-headers
        - name: rate-limit-strict  # Stricter rate limiting for auth
  tls:
    secretName: auth-gateway-tls
```

### 4.5 Registry (Internal Only)

```yaml
# ingress/registry-ingress.yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: registry
  namespace: holm
spec:
  entryPoints:
    - websecure
  routes:
    - match: Host(`registry.holmos.example.com`)
      kind: Rule
      services:
        - name: registry
          port: 5000
      middlewares:
        - name: security-headers
        - name: ip-whitelist  # Restrict to internal IPs
  tls:
    secretName: registry-tls
```

### 4.6 Complete Services Ingress (Path-Based Routing)

```yaml
# ingress/services-ingress.yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: holmos-services
  namespace: holm
spec:
  entryPoints:
    - websecure
  routes:
    # Main shell
    - match: Host(`holmos.example.com`) && PathPrefix(`/`)
      kind: Rule
      priority: 1
      services:
        - name: holmos-shell
          port: 80
    # App Store
    - match: Host(`holmos.example.com`) && PathPrefix(`/apps`)
      kind: Rule
      priority: 10
      services:
        - name: app-store-ai
          port: 80
      middlewares:
        - name: strip-prefix-apps
    # Chat Hub
    - match: Host(`holmos.example.com`) && PathPrefix(`/chat`)
      kind: Rule
      priority: 10
      services:
        - name: chat-hub
          port: 80
      middlewares:
        - name: strip-prefix-chat
    # Nova AI
    - match: Host(`holmos.example.com`) && PathPrefix(`/nova`)
      kind: Rule
      priority: 10
      services:
        - name: nova
          port: 80
      middlewares:
        - name: strip-prefix-nova
    # File Manager
    - match: Host(`holmos.example.com`) && PathPrefix(`/files`)
      kind: Rule
      priority: 10
      services:
        - name: file-web-nautilus
          port: 80
      middlewares:
        - name: strip-prefix-files
    # Terminal
    - match: Host(`holmos.example.com`) && PathPrefix(`/terminal`)
      kind: Rule
      priority: 10
      services:
        - name: terminal-web
          port: 80
      middlewares:
        - name: strip-prefix-terminal
    # Settings
    - match: Host(`holmos.example.com`) && PathPrefix(`/settings`)
      kind: Rule
      priority: 10
      services:
        - name: settings-web
          port: 8080
      middlewares:
        - name: strip-prefix-settings
    # Metrics Dashboard
    - match: Host(`holmos.example.com`) && PathPrefix(`/metrics`)
      kind: Rule
      priority: 10
      services:
        - name: metrics-dashboard
          port: 8080
      middlewares:
        - name: strip-prefix-metrics
  tls:
    secretName: holmos-wildcard-tls
```

---

## 5. Security Configuration

### 5.1 Rate Limiting Middleware

```yaml
# middlewares/rate-limit.yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: rate-limit
  namespace: holm
spec:
  rateLimit:
    average: 100       # requests per second
    burst: 200         # max burst
    period: 1s
    sourceCriterion:
      ipStrategy:
        depth: 1       # Use X-Forwarded-For header
---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: rate-limit-strict
  namespace: holm
spec:
  rateLimit:
    average: 10        # Much stricter for auth endpoints
    burst: 20
    period: 1s
    sourceCriterion:
      ipStrategy:
        depth: 1
```

### 5.2 Security Headers Middleware

```yaml
# middlewares/security-headers.yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: security-headers
  namespace: holm
spec:
  headers:
    # HSTS
    stsSeconds: 31536000
    stsIncludeSubdomains: true
    stsPreload: true
    # Frame Options
    frameDeny: true
    # Content Type Options
    contentTypeNosniff: true
    # XSS Protection
    browserXssFilter: true
    # Referrer Policy
    referrerPolicy: "strict-origin-when-cross-origin"
    # Content Security Policy
    contentSecurityPolicy: |
      default-src 'self';
      script-src 'self' 'unsafe-inline' 'unsafe-eval';
      style-src 'self' 'unsafe-inline';
      img-src 'self' data: https:;
      font-src 'self';
      connect-src 'self' wss: https:;
      frame-ancestors 'none';
    # Permissions Policy
    permissionsPolicy: "camera=(), microphone=(), geolocation=()"
    # Custom Headers
    customResponseHeaders:
      X-Robots-Tag: "noindex, nofollow"
      X-Download-Options: "noopen"
      X-Permitted-Cross-Domain-Policies: "none"
```

### 5.3 IP Whitelist Middleware

```yaml
# middlewares/ip-whitelist.yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: ip-whitelist
  namespace: holm
spec:
  ipAllowList:
    sourceRange:
      - "192.168.8.0/24"    # Local network
      - "10.42.0.0/16"      # Kubernetes pod network
      - "10.43.0.0/16"      # Kubernetes service network
```

### 5.4 Basic WAF with Plugin (Advanced)

```yaml
# middlewares/waf-basic.yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: waf-basic
  namespace: holm
spec:
  plugin:
    traefik-modsecurity-plugin:
      modSecurityUrl: "http://modsecurity:80"
      maxBodySize: 10485760  # 10MB
```

**ModSecurity Deployment:**
```yaml
# security/modsecurity.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: modsecurity
  namespace: holm
spec:
  replicas: 2
  selector:
    matchLabels:
      app: modsecurity
  template:
    metadata:
      labels:
        app: modsecurity
    spec:
      containers:
      - name: modsecurity
        image: owasp/modsecurity-crs:nginx-alpine
        ports:
        - containerPort: 80
        env:
        - name: PARANOIA
          value: "1"  # Level 1-4, higher = more strict
        - name: ANOMALY_INBOUND
          value: "5"
        - name: ANOMALY_OUTBOUND
          value: "4"
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "200m"
---
apiVersion: v1
kind: Service
metadata:
  name: modsecurity
  namespace: holm
spec:
  selector:
    app: modsecurity
  ports:
  - port: 80
    targetPort: 80
```

### 5.5 Forward Authentication Middleware

```yaml
# middlewares/auth-forward.yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: auth-forward
  namespace: holm
spec:
  forwardAuth:
    address: "http://auth-gateway-nodeport.holm.svc.cluster.local/verify"
    trustForwardHeader: true
    authResponseHeaders:
      - X-User-Id
      - X-User-Role
      - X-User-Email
```

### 5.6 Strip Prefix Middlewares

```yaml
# middlewares/strip-prefixes.yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: strip-prefix-apps
  namespace: holm
spec:
  stripPrefix:
    prefixes:
      - /apps
---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: strip-prefix-chat
  namespace: holm
spec:
  stripPrefix:
    prefixes:
      - /chat
---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: strip-prefix-nova
  namespace: holm
spec:
  stripPrefix:
    prefixes:
      - /nova
---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: strip-prefix-files
  namespace: holm
spec:
  stripPrefix:
    prefixes:
      - /files
---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: strip-prefix-terminal
  namespace: holm
spec:
  stripPrefix:
    prefixes:
      - /terminal
---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: strip-prefix-settings
  namespace: holm
spec:
  stripPrefix:
    prefixes:
      - /settings
---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: strip-prefix-metrics
  namespace: holm
spec:
  stripPrefix:
    prefixes:
      - /metrics
```

---

## 6. DNS Considerations

### 6.1 Internal DNS (Local Network)

**Option A: Router DNS / Pi-hole / AdGuard Home**
```
# Add these DNS records to your local DNS server
holmos.example.com      A    192.168.8.197  # Control plane IP
*.holmos.example.com    A    192.168.8.197  # Wildcard for subdomains
```

**Option B: /etc/hosts (Development)**
```bash
# Add to /etc/hosts on client machines
192.168.8.197  holmos.example.com
192.168.8.197  auth.holmos.example.com
192.168.8.197  nova.holmos.example.com
192.168.8.197  registry.holmos.example.com
```

**Option C: CoreDNS ConfigMap (Cluster-internal)**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns-custom
  namespace: kube-system
data:
  holmos.server: |
    holmos.local:53 {
        hosts {
            192.168.8.197 holmos.local
            192.168.8.197 auth.holmos.local
            192.168.8.197 nova.holmos.local
            fallthrough
        }
    }
```

### 6.2 External DNS (Public Access)

**Requirements for Public Access:**
1. Domain name (e.g., `holmos.yourdomain.com`)
2. Public IP or Dynamic DNS service
3. Port forwarding on router (80, 443 to cluster)

**Cloudflare DNS Example:**
```
Type    Name                    Content         Proxy
A       holmos                  YOUR_PUBLIC_IP  Proxied
CNAME   *.holmos                holmos          Proxied
```

**Dynamic DNS Services:**
- DuckDNS (free)
- Cloudflare DDNS
- No-IP

### 6.3 Split-Horizon DNS

For different resolution based on location:

```yaml
# External DNS automation
apiVersion: externaldns.k8s.io/v1alpha1
kind: DNSEndpoint
metadata:
  name: holmos-external
  namespace: holm
spec:
  endpoints:
  - dnsName: holmos.example.com
    recordType: A
    targets:
    - YOUR_EXTERNAL_IP
```

---

## 7. Implementation Plan

### Phase 1: Foundation (Week 1)

1. **Install cert-manager**
   ```bash
   kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml
   ```

2. **Create self-signed CA for development**
   ```bash
   kubectl apply -f cert-manager/selfsigned-issuer.yaml
   ```

3. **Create base middlewares**
   ```bash
   kubectl apply -f middlewares/security-headers.yaml
   kubectl apply -f middlewares/rate-limit.yaml
   kubectl apply -f middlewares/ip-whitelist.yaml
   ```

4. **Fix existing auth-gateway ingress**
   ```bash
   kubectl delete ingress auth-gateway -n holm
   kubectl apply -f ingress/auth-gateway-ingress.yaml
   ```

### Phase 2: Core Services (Week 2)

1. **Create IngressRoutes for critical services**
   - holmos-shell
   - auth-gateway
   - nova

2. **Test internal access with self-signed certs**

3. **Set up local DNS records**

### Phase 3: Full Migration (Week 3)

1. **Create IngressRoutes for all services**

2. **Remove NodePort services (or keep as backup)**
   ```bash
   # Convert to ClusterIP
   kubectl patch svc holmos-shell -n holm -p '{"spec":{"type":"ClusterIP"}}'
   ```

3. **Test all endpoints**

### Phase 4: Production Hardening (Week 4)

1. **Configure Let's Encrypt production issuer**

2. **Enable WAF (ModSecurity)**

3. **Set up monitoring and alerting**

4. **Document runbooks**

---

## 8. Monitoring and Observability

### 8.1 Traefik Dashboard Access

```yaml
# ingress/traefik-dashboard.yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: traefik-dashboard
  namespace: kube-system
spec:
  entryPoints:
    - websecure
  routes:
    - match: Host(`traefik.holmos.example.com`)
      kind: Rule
      services:
        - name: api@internal
          kind: TraefikService
      middlewares:
        - name: ip-whitelist
          namespace: holm
  tls:
    secretName: traefik-dashboard-tls
```

### 8.2 Prometheus Metrics

Traefik already exposes metrics on port 9100. Add ServiceMonitor:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: traefik
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: traefik
  endpoints:
  - port: metrics
    interval: 30s
```

### 8.3 Access Logging

```yaml
# Update Traefik deployment args
- --accesslog=true
- --accesslog.format=json
- --accesslog.fields.defaultmode=keep
- --accesslog.fields.headers.defaultmode=keep
```

---

## 9. Troubleshooting

### Common Issues

1. **Certificate not issuing**
   ```bash
   kubectl describe certificate <cert-name> -n <namespace>
   kubectl describe certificaterequest -n <namespace>
   kubectl logs -n cert-manager deploy/cert-manager
   ```

2. **503 Service Unavailable**
   ```bash
   kubectl get endpoints <service-name> -n <namespace>
   kubectl describe ingressroute <name> -n <namespace>
   ```

3. **SSL/TLS errors**
   ```bash
   curl -vk https://holmos.example.com
   openssl s_client -connect holmos.example.com:443
   ```

4. **Rate limiting too aggressive**
   - Check middleware configuration
   - Temporarily disable rate limiting for debugging

---

## 10. Security Checklist

- [ ] TLS 1.2+ enforced
- [ ] HSTS headers enabled
- [ ] Rate limiting configured
- [ ] Security headers applied
- [ ] Internal services IP-restricted
- [ ] Forward authentication enabled where needed
- [ ] WAF rules deployed
- [ ] Access logs enabled
- [ ] Certificate auto-renewal tested
- [ ] DNS records configured
- [ ] Backup ingress method documented

---

## Appendix A: Quick Commands

```bash
# Check Traefik status
kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik

# View Traefik logs
kubectl logs -n kube-system -l app.kubernetes.io/name=traefik -f

# List all ingress resources
kubectl get ingress,ingressroute --all-namespaces

# Check certificates
kubectl get certificates --all-namespaces

# Test endpoint
curl -H "Host: holmos.example.com" http://192.168.8.197/

# View middleware applied
kubectl get middlewares.traefik.io --all-namespaces
```

---

## Appendix B: Service to Subdomain Mapping

| Service | Recommended Subdomain | Priority |
|---------|----------------------|----------|
| holmos-shell | holmos.example.com | High |
| auth-gateway | auth.holmos.example.com | High |
| nova | nova.holmos.example.com | High |
| chat-hub | chat.holmos.example.com | Medium |
| file-web-nautilus | files.holmos.example.com | Medium |
| terminal-web | terminal.holmos.example.com | Medium |
| metrics-dashboard | metrics.holmos.example.com | Medium |
| registry | registry.holmos.example.com | High (internal) |
| registry-ui | registry-ui.holmos.example.com | Low |
| vault | vault.holmos.example.com | High (internal) |
| settings-web | settings.holmos.example.com | Low |
| app-store-ai | apps.holmos.example.com | Medium |
| cluster-manager | cluster.holmos.example.com | Low (internal) |
| traefik dashboard | traefik.holmos.example.com | Low (internal) |
