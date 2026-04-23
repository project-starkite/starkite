#!/usr/bin/env kite
# wordpress-stack.star - WordPress + MySQL deployment with optional Ingress
#
# Replaces: helm install wordpress bitnami/wordpress
#
# Script-driven deployment — k.apply(), k.wait_for(), printf() status.
# No piping, no kubectl. Applies resources in dependency order, waits for
# each Deployment to become available, and prints access instructions.
#
# Usage (imperative — applies directly to cluster):
#   kite run examples/cloud/wordpress-stack/wordpress-stack.star
#   kite run examples/cloud/wordpress-stack/wordpress-stack.star --var domain=blog.example.com --var tls=true
#   kite run examples/cloud/wordpress-stack/wordpress-stack.star --var persistence=false
#   kite run examples/cloud/wordpress-stack/wordpress-stack.star --dry-run
#
# Usage (var-file):
#   kite run examples/cloud/wordpress-stack/wordpress-stack.star --var-file values.yaml

def build_secret(db_name, db_user, db_password, db_root_password):
    """Build a Secret with database credentials."""
    return k8s.obj.secret(
        name="wordpress-db-credentials",
        string_data={
            "MYSQL_DATABASE": db_name,
            "MYSQL_USER": db_user,
            "MYSQL_PASSWORD": db_password,
            "MYSQL_ROOT_PASSWORD": db_root_password,
            "WORDPRESS_DB_HOST": "wordpress-db",
            "WORDPRESS_DB_USER": db_user,
            "WORDPRESS_DB_PASSWORD": db_password,
            "WORDPRESS_DB_NAME": db_name,
        },
    )

def build_pvc(name, storage_size, storage_class):
    """Build a PersistentVolumeClaim."""
    pvc = k8s.obj.persistent_volume_claim(
        name=name,
        access_modes=["ReadWriteOnce"],
        storage=storage_size,
    )
    return pvc

def build_mysql(image, persistence, storage_class, db_storage):
    """Build MySQL Deployment + Service."""
    volumes = []
    volume_mounts = []
    if persistence:
        volume_mounts = [
            k8s.obj.volume_mount(name="db-data", mount_path="/var/lib/mysql"),
        ]
        volumes = [
            k8s.obj.volume(name="db-data", pvc={"claimName": "wordpress-db-data"}),
        ]
    else:
        volume_mounts = [
            k8s.obj.volume_mount(name="db-data", mount_path="/var/lib/mysql"),
        ]
        volumes = [
            k8s.obj.volume(name="db-data", empty_dir={}),
        ]

    container = k8s.obj.container(
        name="mysql",
        image=image,
        ports=[k8s.obj.container_port(container_port=3306, name="mysql")],
        env_from=[
            k8s.obj.env_from(secret_ref={"name": "wordpress-db-credentials"}),
        ],
        volume_mounts=volume_mounts,
        readiness_probe=k8s.obj.probe(
            tcp_socket={"port": 3306},
            initial_delay_seconds=15,
            period_seconds=10,
        ),
        liveness_probe=k8s.obj.probe(
            tcp_socket={"port": 3306},
            initial_delay_seconds=30,
            period_seconds=20,
        ),
        resources=k8s.obj.resource_requirements(
            requests={"cpu": "250m", "memory": "256Mi"},
            limits={"cpu": "1", "memory": "1Gi"},
        ),
    )

    dep = k8s.obj.deployment(
        name="wordpress-db",
        labels={"app": "wordpress", "component": "db"},
        replicas=1,
        containers=[container],
        volumes=volumes,
    )

    svc = k8s.obj.service(
        name="wordpress-db",
        selector={"app": "wordpress", "component": "db"},
        ports=[k8s.obj.service_port(name="mysql", port=3306, target_port=3306)],
    )

    return dep, svc

def build_wordpress(image, persistence, storage_class, wp_storage):
    """Build WordPress Deployment + Service."""
    volumes = []
    volume_mounts = []
    if persistence:
        volume_mounts = [
            k8s.obj.volume_mount(name="wp-data", mount_path="/var/www/html"),
        ]
        volumes = [
            k8s.obj.volume(name="wp-data", pvc={"claimName": "wordpress-data"}),
        ]
    else:
        volume_mounts = [
            k8s.obj.volume_mount(name="wp-data", mount_path="/var/www/html"),
        ]
        volumes = [
            k8s.obj.volume(name="wp-data", empty_dir={}),
        ]

    container = k8s.obj.container(
        name="wordpress",
        image=image,
        ports=[k8s.obj.container_port(container_port=80, name="http")],
        env_from=[
            k8s.obj.env_from(secret_ref={"name": "wordpress-db-credentials"}),
        ],
        volume_mounts=volume_mounts,
        readiness_probe=k8s.obj.probe(
            http_get={"path": "/wp-login.php", "port": 80},
            initial_delay_seconds=30,
            period_seconds=10,
        ),
        liveness_probe=k8s.obj.probe(
            http_get={"path": "/wp-login.php", "port": 80},
            initial_delay_seconds=60,
            period_seconds=30,
            timeout_seconds=5,
        ),
        resources=k8s.obj.resource_requirements(
            requests={"cpu": "250m", "memory": "256Mi"},
            limits={"cpu": "1", "memory": "512Mi"},
        ),
    )

    dep = k8s.obj.deployment(
        name="wordpress",
        labels={"app": "wordpress", "component": "web"},
        replicas=1,
        containers=[container],
        volumes=volumes,
    )

    svc = k8s.obj.service(
        name="wordpress",
        type="ClusterIP",
        selector={"app": "wordpress", "component": "web"},
        ports=[k8s.obj.service_port(name="http", port=80, target_port=80)],
    )

    return dep, svc

def build_ingress(domain, tls):
    """Build an Ingress for the WordPress frontend."""
    ingress_path = k8s.obj.ingress_path(
        path="/",
        path_type="Prefix",
        backend={"service": {"name": "wordpress", "port": {"number": 80}}},
    )

    rule = k8s.obj.ingress_rule(
        host=domain,
        paths={"paths": [ingress_path]},
    )

    tls_config = []
    if tls:
        tls_config = [{"hosts": [domain], "secretName": "wordpress-tls"}]

    return k8s.obj.ingress(
        name="wordpress",
        ingress_class_name="nginx",
        rules=[rule],
        tls=tls_config if tls else [],
    )

def main():
    # --- Variables ---------------------------------------------------------------
    image = var_str("image", "wordpress:6-apache")
    db_image = var_str("db.image", "mysql:8.0")
    domain = var_str("domain", "")
    tls = var_bool("tls", False)
    persistence = var_bool("persistence", True)
    storage_class = var_str("storage.class", "")
    wp_storage = var_str("wp.storage", "10Gi")
    db_storage = var_str("db.storage", "10Gi")
    db_name = var_str("db.name", "wordpress")
    db_user = var_str("db.user", "wordpress")
    db_password = var_str("db.password", "changeme")
    db_root_password = var_str("db.root.password", "changeme")
    ns = var_str("namespace", "default")

    k = k8s.config(namespace=ns)

    # --- Build all resources -----------------------------------------------------
    secret = build_secret(db_name, db_user, db_password, db_root_password)
    db_dep, db_svc = build_mysql(db_image, persistence, storage_class, db_storage)
    wp_dep, wp_svc = build_wordpress(image, persistence, storage_class, wp_storage)

    # --- Step 1: Create credentials ----------------------------------------------
    printf("Applying credentials to namespace %s...\n", ns)
    k.apply([secret])

    # --- Step 2: Create PVCs (if persistence enabled) ----------------------------
    if persistence:
        wp_pvc = build_pvc("wordpress-data", wp_storage, storage_class)
        db_pvc = build_pvc("wordpress-db-data", db_storage, storage_class)
        printf("Creating persistent volumes...\n")
        k.apply([wp_pvc, db_pvc])

    # --- Step 3: Deploy MySQL and wait -------------------------------------------
    printf("Deploying MySQL...\n")
    k.apply([db_dep, db_svc])
    result = k.wait_for("deployment", "wordpress-db", condition="available", timeout="2m")
    if not result["ready"]:
        printf("MySQL did not become ready: %s\n", result["message"])
        fail("mysql deployment failed")
    printf("MySQL is ready.\n")

    # --- Step 4: Deploy WordPress and wait ---------------------------------------
    printf("Deploying WordPress...\n")
    k.apply([wp_dep, wp_svc])
    result = k.wait_for("deployment", "wordpress", condition="available", timeout="3m")
    if not result["ready"]:
        printf("WordPress did not become ready: %s\n", result["message"])
        fail("wordpress deployment failed")
    printf("WordPress is ready.\n")

    # --- Step 5: Ingress (conditional) -------------------------------------------
    if domain:
        ingress = build_ingress(domain, tls)
        printf("Creating Ingress for %s...\n", domain)
        k.apply([ingress])

    # --- Summary -----------------------------------------------------------------
    printf("\n--- WordPress Stack Deployed ---\n")
    printf("  Namespace:   %s\n", ns)
    printf("  WordPress:   %s\n", image)
    printf("  MySQL:       %s\n", db_image)
    printf("  Persistence: %s\n", "true" if persistence else "false")
    if domain:
        scheme = "https" if tls else "http"
        printf("  URL:         %s://%s\n", scheme, domain)
    else:
        printf("  Access:      kubectl port-forward svc/wordpress 8080:80 -n %s\n", ns)
        printf("               then open http://localhost:8080\n")

main()
