# WordPress Stack

Replaces: `helm install wordpress bitnami/wordpress`

Script-driven WordPress + MySQL deployment with optional Ingress and TLS.
Uses `k.apply()` and `k.wait_for()` to apply resources in dependency order —
no piping, no kubectl.

## What it demonstrates

- **Imperative deployment** — `k.apply()`, `k.wait_for()`, `printf()` status
- Dependency ordering: Secret -> PVCs -> MySQL -> WordPress -> Ingress
- Conditional resources: Ingress only when `domain` is set, PVCs only when `persistence=true`
- `k8s.obj.secret()` with `string_data` for database credentials
- `k8s.obj.persistent_volume_claim()` with `storage` shorthand

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `image` | No | `wordpress:6-apache` | WordPress image |
| `db.image` | No | `mysql:8.0` | MySQL image |
| `domain` | No | *(empty = no Ingress)* | Ingress hostname |
| `tls` | No | `false` | Enable TLS on Ingress |
| `persistence` | No | `true` | Enable PVCs |
| `storage.class` | No | *(empty)* | StorageClass name |
| `wp.storage` | No | `10Gi` | WordPress PVC size |
| `db.storage` | No | `10Gi` | MySQL PVC size |
| `db.name` | No | `wordpress` | Database name |
| `db.user` | No | `wordpress` | Database user |
| `db.password` | No | `changeme` | Database password |
| `db.root.password` | No | `changeme` | MySQL root password |
| `namespace` | No | `default` | Target namespace |

## Usage

```bash
# Deploy with defaults
kite run examples/cloud/wordpress-stack/wordpress-stack.star

# Deploy with custom domain and TLS
kite run examples/cloud/wordpress-stack/wordpress-stack.star --var domain=blog.example.com --var tls=true

# Deploy without persistence (emptyDir)
kite run examples/cloud/wordpress-stack/wordpress-stack.star --var persistence=false

# Dry-run — preview without applying
kite run examples/cloud/wordpress-stack/wordpress-stack.star --dry-run
```

## What it creates

- **Secret** — `wordpress-db-credentials` with MySQL and WordPress connection strings
- **PersistentVolumeClaim** x2 — `wordpress-data` (10Gi) + `wordpress-db-data` (10Gi), conditional
- **MySQL Deployment** — single replica with readiness/liveness probes
- **MySQL Service** — ClusterIP on port 3306
- **WordPress Deployment** — single replica with probes and envFrom (Secret)
- **WordPress Service** — ClusterIP on port 80
- **Ingress** — conditional, with optional TLS

## How it works

1. Builds all resources using `k8s.obj.*` constructors
2. Applies credentials (`k.apply([secret])`)
3. Creates PVCs if persistence enabled
4. Deploys MySQL and waits for available condition (`k.wait_for()`)
5. Deploys WordPress and waits for available condition
6. Creates Ingress if `domain` is set
7. Prints summary with access instructions (URL or port-forward)
