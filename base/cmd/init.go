package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	initTemplate  string
	listTemplates bool
)

var initCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Initialize a new starkite project",
	Long: `Initialize a new starkite project with configuration files and optional templates.

Templates:
  basic      - Minimal config.yaml only (default)
  deployment - SSH deployment script with inventory
  kubernetes - Kubernetes manifest generation
  backup     - Remote backup collection script

Examples:
  # Initialize in current directory
  kite init

  # Initialize with deployment template
  kite init --template=deployment

  # Initialize in specific directory
  kite init ./my-project

  # List available templates
  kite init --list-templates
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initTemplate, "template", "t", "basic", "Project template to use")
	initCmd.Flags().BoolVar(&listTemplates, "list-templates", false, "List available templates")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	if listTemplates {
		fmt.Println("Available templates:")
		fmt.Println("  basic       - Minimal starkite.yaml only (default)")
		fmt.Println("  deployment  - SSH deployment script with inventory")
		fmt.Println("  kubernetes  - Kubernetes manifest generation")
		fmt.Println("  backup      - Remote backup collection script")
		return nil
	}

	// Determine target directory
	dir := "."
	if len(args) > 0 {
		dir = args[0]
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Get template files
	tmpl, ok := templates[initTemplate]
	if !ok {
		return fmt.Errorf("unknown template: %s (use --list-templates to see available)", initTemplate)
	}

	// Create files
	for _, file := range tmpl {
		path := filepath.Join(dir, file.Name)
		
		// Check if file already exists
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("Skipped %s (already exists)\n", path)
			continue
		}

		if err := os.WriteFile(path, []byte(file.Content), 0644); err != nil {
			return fmt.Errorf("failed to create %s: %w", path, err)
		}
		fmt.Printf("Created %s\n", path)
	}

	return nil
}

// TemplateFile represents a file to create
type TemplateFile struct {
	Name    string
	Content string
}

var templates = map[string][]TemplateFile{
	"basic": {
		{
			Name: "config.yaml",
			Content: `# starkite project configuration
# See: https://github.com/project-starkite/starkite

project:
  name: my-project
  version: 0.1.0

defaults:
  log_level: info
  timeout: 300

providers:
  ssh:
    # user: deploy
    # private_key_file: ~/.ssh/id_rsa

# Variables
# environment: dev
# replicas: 3
`,
		},
	},
	"deployment": {
		{
			Name: "config.yaml",
			Content: `# starkite project configuration

project:
  name: my-deployment
  version: 0.1.0

defaults:
  log_level: info
  timeout: 300

providers:
  ssh:
    user: deploy
    private_key_file: ~/.ssh/id_rsa

# Variables
environment: dev
app_name: myapp
`,
		},
		{
			Name: "deploy.star",
			Content: `#!/usr/bin/env kite run
# deploy.star - Deployment script
#
# Usage:
#   kite run deploy.star
#   kite run deploy.star --var environment=prod

# Load inventory
hosts = inventory.file("hosts.yaml", port = 22)

# Get configuration
env = var("environment", "dev")
app = var("app_name", "myapp")

log.info("Starting deployment", app = app, environment = env)

# Filter hosts by environment
targets = inventory.filter(hosts, group = env)
if len(targets) == 0:
    fail("No hosts found for environment: " + env)

log.info("Target hosts", count = len(targets))

# Configure SSH
ssh_client = ssh.config(
    user = var("ssh.user", "deploy"),
    private_key_file = var("ssh.private_key_file", "~/.ssh/id_rsa"),
    host_list = inventory.addresses(targets)
)

# Deploy
results = ssh_client.exec("echo 'Deploying %s to %s'" % (app, env))

for r in results:
    if r.err:
        log.error("Failed", host = r.host, error = r.err)
    else:
        log.info("Success", host = r.host, output = r.value.strip())

log.info("Deployment complete")
`,
		},
		{
			Name: "hosts.yaml",
			Content: `# Inventory file
# Format: address, name (optional), group (optional)

# Development hosts
- address: 10.0.1.10
  name: dev-web-1
  group: dev

# Staging hosts
# - address: staging.example.com
#   name: staging-web-1
#   group: staging

# Production hosts
# - address: prod-web-1.example.com
#   name: prod-web-1
#   group: prod
# - address: prod-web-2.example.com
#   name: prod-web-2
#   group: prod
`,
		},
	},
	"kubernetes": {
		{
			Name: "config.yaml",
			Content: `# starkite project configuration

project:
  name: k8s-manifests
  version: 0.1.0

defaults:
  log_level: info

# Variables
namespace: default
app_name: myapp
image_tag: latest
replicas: 3
`,
		},
		{
			Name: "manifests.star",
			Content: `#!/usr/bin/env kite run
# manifests.star - Generate Kubernetes manifests
#
# Usage:
#   kite run manifests.star                           # Print YAML
#   kite run manifests.star | kubectl apply -f -      # Apply to cluster
#   kite run manifests.star --var image_tag=v1.0.0    # Custom version

# Configuration
namespace = var("namespace", "default")
app_name = var("app_name", "myapp")
image_tag = var("image_tag", "latest")
replicas = int(var("replicas", "3"))

# Helper functions
def labels():
    return {
        "app.kubernetes.io/name": app_name,
        "app.kubernetes.io/managed-by": "starkite"
    }

# Deployment manifest
deployment = {
    "apiVersion": "apps/v1",
    "kind": "Deployment",
    "metadata": {
        "name": app_name,
        "namespace": namespace,
        "labels": labels()
    },
    "spec": {
        "replicas": replicas,
        "selector": {
            "matchLabels": {"app.kubernetes.io/name": app_name}
        },
        "template": {
            "metadata": {
                "labels": labels()
            },
            "spec": {
                "containers": [{
                    "name": app_name,
                    "image": "myregistry.io/%s:%s" % (app_name, image_tag),
                    "ports": [{"containerPort": 8080}],
                    "resources": {
                        "requests": {"cpu": "100m", "memory": "128Mi"},
                        "limits": {"cpu": "500m", "memory": "512Mi"}
                    }
                }]
            }
        }
    }
}

# Service manifest
service = {
    "apiVersion": "v1",
    "kind": "Service",
    "metadata": {
        "name": app_name,
        "namespace": namespace,
        "labels": labels()
    },
    "spec": {
        "selector": {"app.kubernetes.io/name": app_name},
        "ports": [{"port": 80, "targetPort": 8080}],
        "type": "ClusterIP"
    }
}

# Output YAML
print("---")
print(yaml.encode(deployment))
print("---")
print(yaml.encode(service))
`,
		},
	},
	"backup": {
		{
			Name: "config.yaml",
			Content: `# starkite project configuration

project:
  name: backup-collector
  version: 0.1.0

defaults:
  log_level: info
  timeout: 600

providers:
  ssh:
    user: backup
    private_key_file: ~/.ssh/backup_key

# Variables
backup_dir: /tmp/backups
`,
		},
		{
			Name: "backup.star",
			Content: `#!/usr/bin/env kite run
# backup.star - Collect backups from remote servers
#
# Usage:
#   kite run backup.star
#   kite run backup.star --var-file=prod-hosts.yaml

# Configuration
backup_dir = var("backup_dir", "/tmp/backups")
timestamp = time.format(time.now(), "2006-01-02-150405")

log.info("Starting backup collection", timestamp = timestamp)

# Load inventory
hosts = inventory.file("hosts.yaml", port = 22)

# Create local backup directory
os.exec("mkdir -p " + backup_dir)

# Configure SSH
ssh_client = ssh.config(
    user = var("ssh.user", "backup"),
    private_key_file = var("ssh.private_key_file", "~/.ssh/backup_key"),
    host_list = inventory.addresses(hosts)
)

# Collect logs from each host
log.info("Collecting logs from hosts", count = len(hosts))

results = ssh_client.exec("cat /var/log/syslog | tail -1000")

for r in results:
    if r.err:
        log.error("Failed to collect", host = r.host, error = r.err)
    else:
        # Save to local file
        filename = "%s/%s-%s.log" % (backup_dir, r.host, timestamp)
        # Replace : with - in filename for Windows compatibility
        filename = strings.replace(filename, ":", "-", -1)
        write_text(filename, r.value)
        log.info("Saved", host = r.host, file = filename)

log.info("Backup collection complete", directory = backup_dir)
`,
		},
		{
			Name: "hosts.yaml",
			Content: `# Backup targets
- address: server1.example.com
  name: server1
  group: web

- address: server2.example.com
  name: server2
  group: web
`,
		},
	},
}
