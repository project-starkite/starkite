package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	initName      string
	initTemplate  string
	listTemplates bool
)

var initCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Scaffold a new starkite module",
	Long: `Scaffold a new starkite module: a directory with main.star, mod.yaml,
mod.lock, and README.md.

The module's name defaults to the target directory's name; override it with
--name (accepts "name" or "namespace/name"). A --template adds an example
main.star and any supporting files on top of the base scaffold.

Templates:
  basic       Minimal runnable module (default)
  kubernetes  Kubernetes manifest generation

Examples:
  # Scaffold in the current directory
  kite init

  # Scaffold in ./my-module with an explicit identity
  kite init ./my-module --name acme/my-module

  # Scaffold with a template
  kite init --template=kubernetes

  # List available templates
  kite init --list-templates
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initName, "name", "", "Module identity: \"name\" or \"namespace/name\"")
	initCmd.Flags().StringVarP(&initTemplate, "template", "t", "basic", "Template overlay to apply")
	initCmd.Flags().BoolVar(&listTemplates, "list-templates", false, "List available templates")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	if listTemplates {
		fmt.Println("Available templates:")
		fmt.Println("  basic       Minimal runnable module (default)")
		fmt.Println("  kubernetes  Kubernetes manifest generation")
		return nil
	}

	dir := "."
	if len(args) > 0 {
		dir = args[0]
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	overlay, ok := templateOverlays[initTemplate]
	if !ok {
		return fmt.Errorf("unknown template %q (use --list-templates to see available)", initTemplate)
	}

	namespace, name := moduleIdentity(initName, dir)

	files := []TemplateFile{
		{Name: "mod.yaml", Content: modManifest(namespace, name)},
		{Name: "main.star", Content: overlay.MainStar},
		{Name: "mod.lock", Content: "version: 1\n"},
		{Name: "README.md", Content: moduleReadme(namespace, name)},
	}
	files = append(files, overlay.Extras...)

	for _, file := range files {
		path := filepath.Join(dir, file.Name)
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("Skipped %s (already exists)\n", path)
			continue
		}
		if err := os.WriteFile(path, []byte(file.Content), 0o644); err != nil {
			return fmt.Errorf("failed to create %s: %w", path, err)
		}
		fmt.Printf("Created %s\n", path)
	}

	return nil
}

// moduleIdentity resolves the module's (namespace, name). An explicit --name of
// the form "namespace/name" sets both; a bare "name" sets only the name.
// Without --name, the name defaults to the target directory's base name.
func moduleIdentity(nameFlag, dir string) (namespace, name string) {
	if nameFlag != "" {
		if ns, n, ok := strings.Cut(nameFlag, "/"); ok {
			return ns, n
		}
		return "", nameFlag
	}
	base := "module"
	if abs, err := filepath.Abs(dir); err == nil {
		base = filepath.Base(abs)
	}
	return "", sanitizeName(base)
}

// sanitizeName lower-cases base and replaces spaces with hyphens, falling back
// to "module" when nothing usable remains.
func sanitizeName(base string) string {
	name := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(base), " ", "-"))
	if name == "" || name == "." || name == ".." {
		return "module"
	}
	return name
}

// modManifest renders a mod.yaml with the required name (and namespace when set),
// plus commented-out optional fields.
func modManifest(namespace, name string) string {
	var b strings.Builder
	if namespace != "" {
		fmt.Fprintf(&b, "namespace: %s\n", namespace)
	}
	fmt.Fprintf(&b, "name: %s\n", name)
	b.WriteString("version: 0.1.0\n")
	b.WriteString("\n")
	b.WriteString("# description: A short summary of this module.\n")
	b.WriteString("# dependencies:\n")
	b.WriteString("#   acme/leaf: gitlab.com/acme/leaf@v1.0.0\n")
	return b.String()
}

// moduleReadme renders the module's README.md.
func moduleReadme(namespace, name string) string {
	identity := name
	if namespace != "" {
		identity = namespace + "/" + name
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", identity)
	b.WriteString("A starkite module.\n\n")
	b.WriteString("## Run\n\n")
	b.WriteString("```bash\n")
	b.WriteString("kite run .\n")
	b.WriteString("```\n\n")
	b.WriteString("## Layout\n\n")
	b.WriteString("- `main.star` — entry point; defines `main()`\n")
	b.WriteString("- `mod.yaml` — identity and declared dependencies\n")
	b.WriteString("- `mod.lock` — resolved dependency lockfile (generated; commit it)\n")
	return b.String()
}

// TemplateFile represents a file to create.
type TemplateFile struct {
	Name    string
	Content string
}

// templateOverlay is the per-template content layered on the base module
// scaffold: the main.star body and any supporting files.
type templateOverlay struct {
	MainStar string
	Extras   []TemplateFile
}

var templateOverlays = map[string]templateOverlay{
	"basic": {
		MainStar: `# main.star — module entry point.

def main():
    print("hello from starkite")
`,
	},
	"kubernetes": {
		MainStar: `# main.star — Kubernetes manifest generation.
#
#   kite run .                        # print YAML
#   kite run . | kubectl apply -f -   # apply to a cluster

def main():
    app = var_str("app_name", "myapp")
    cfg = struct(
        namespace = var_str("namespace", "default"),
        app = app,
        image = "%s:%s" % (app, var_str("image_tag", "latest")),
        replicas = var_int("replicas", 3),
        port = 8080,
        labels = {
            "app.kubernetes.io/name": app,
            "app.kubernetes.io/managed-by": "starkite",
        },
        selector = {"app.kubernetes.io/name": app},
    )

    deployment = k8s.obj.deployment(
        name = cfg.app,
        namespace = cfg.namespace,
        labels = cfg.labels,
        replicas = cfg.replicas,
        selector = {"matchLabels": cfg.selector},
        containers = [
            k8s.obj.container(
                name = cfg.app,
                image = cfg.image,
                ports = [k8s.obj.container_port(container_port = cfg.port)],
            ),
        ],
    )
    service = k8s.obj.service(
        name = cfg.app,
        namespace = cfg.namespace,
        labels = cfg.labels,
        selector = cfg.selector,
        ports = [k8s.obj.service_port(port = 80, target_port = cfg.port)],
    )
    print(k8s.yaml([deployment, service]))
`,
	},
}
