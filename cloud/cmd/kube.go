package commands

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/cloud/loader"
	"github.com/vladimirvivien/starkite/core/varstore"
	"github.com/vladimirvivien/starkite/starbase"
)

//go:embed gen_controller.star
var defaultGeneratorScript string

//go:embed gen_webhook.star
var defaultWebhookGeneratorScript string

var kubeCmd = &cobra.Command{
	Use:   "kube",
	Short: "Kubernetes-specific commands",
	Long:  "Commands for Kubernetes resource generation and management.",
}

var (
	genController string
	genResource   string
	genImage      string
	genNamespace  string
	genReplicas   int
	genOutput     string
	genDockerfile string
	genGenerator  string
)

var genArtifactsCmd = &cobra.Command{
	Use:   "gen-controller-artifacts",
	Short: "Generate Kubernetes deployment artifacts for a controller",
	Long: `Generate all deployment manifests for a starkite controller.

Executes a Starlark generator script that builds manifests using k8s.obj.*
constructors. The default generator produces:
  - CustomResourceDefinition (if --resource provided)
  - Namespace, ServiceAccount, ClusterRole, ClusterRoleBinding
  - Deployment

Use --generator to provide a custom generator script for complex scenarios.

Examples:
  # Generate YAML manifests (default)
  kite-cloud kube gen-controller-artifacts \
      --controller controller.star \
      --resource resource.star \
      --image myregistry/myapp-controller:v1 \
      --namespace myapp-system > deploy.yaml

  # Generate a Starlark deployment script
  kite-cloud kube gen-controller-artifacts \
      --controller controller.star \
      --image myregistry/myapp-controller:v1 \
      --output script > deploy-controller.star

  # Use a custom generator
  kite-cloud kube gen-controller-artifacts \
      --generator my-generator.star \
      --image myregistry/myapp-controller:v1
`,
	RunE: runGenArtifacts,
}

func init() {
	genArtifactsCmd.Flags().StringVar(&genController, "controller", "controller.star", "Path to controller script")
	genArtifactsCmd.Flags().StringVar(&genResource, "resource", "", "Path to resource definition script (for CRD generation)")
	genArtifactsCmd.Flags().StringVar(&genImage, "image", "", "Container image for the controller (required)")
	genArtifactsCmd.Flags().StringVar(&genNamespace, "namespace", "default", "Namespace for the controller deployment")
	genArtifactsCmd.Flags().IntVar(&genReplicas, "replicas", 1, "Number of controller replicas")
	genArtifactsCmd.Flags().StringVar(&genOutput, "output", "yaml", "Output format: yaml or script")
	genArtifactsCmd.Flags().StringVar(&genDockerfile, "dockerfile", "", "Generate Dockerfile with given name (e.g., Dockerfile)")
	genArtifactsCmd.Flags().StringVar(&genGenerator, "generator", "", "Custom generator script (overrides built-in)")

	kubeCmd.AddCommand(genArtifactsCmd)
}

func runGenArtifacts(cmd *cobra.Command, args []string) error {
	if genImage == "" {
		return fmt.Errorf("--image is required")
	}
	if genOutput != "yaml" && genOutput != "script" {
		return fmt.Errorf("--output must be 'yaml' or 'script', got %q", genOutput)
	}

	// Determine generator script source
	var scriptSource string
	var scriptName string
	if genGenerator != "" {
		data, err := os.ReadFile(genGenerator)
		if err != nil {
			return fmt.Errorf("failed to read generator %q: %w", genGenerator, err)
		}
		scriptSource = string(data)
		scriptName = genGenerator
	} else {
		scriptSource = defaultGeneratorScript
		scriptName = "<builtin-generator>"
	}

	// If --resource provided, execute it to extract CRD YAML
	crdYAML := ""
	if genResource != "" {
		yaml, err := extractCRDYAML(genResource)
		if err != nil {
			return fmt.Errorf("failed to process resource file %q: %w", genResource, err)
		}
		crdYAML = yaml
	}

	// Build variable store with CLI flags
	vars := varstore.New()
	vars.Set("controller_script", genController)
	vars.Set("image", genImage)
	vars.Set("namespace", genNamespace)
	vars.Set("replicas", fmt.Sprintf("%d", genReplicas))
	vars.Set("output_format", genOutput)
	vars.Set("crd_yaml", crdYAML)

	// Create cloud registry with module config
	moduleConfig := &starbase.ModuleConfig{
		VarStore: vars,
	}
	registry := loader.NewCloudRegistry(moduleConfig)

	// Create runtime
	rt, err := starbase.New(&starbase.Config{
		Registry:   registry,
		VarStore:   vars,
		ScriptPath: scriptName,
	})
	if err != nil {
		return fmt.Errorf("failed to create runtime: %w", err)
	}

	// Execute the generator script
	if err := rt.Execute(context.Background(), scriptSource); err != nil {
		return fmt.Errorf("generator failed: %w", err)
	}

	// Generate Dockerfile if requested
	if genDockerfile != "" {
		if err := writeDockerfile(genDockerfile); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Dockerfile written to ./%s\n", genDockerfile)
	}

	return nil
}

// extractCRDYAML executes a resource.star file and extracts CRD YAML from it.
// It runs the script, captures all print output, and returns it as the CRD YAML.
func extractCRDYAML(resourceFile string) (string, error) {
	data, err := os.ReadFile(resourceFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", resourceFile, err)
	}

	// Execute resource.star — it should print CRD YAML via k8s.yaml()
	var output strings.Builder
	moduleConfig := &starbase.ModuleConfig{}
	registry := loader.NewCloudRegistry(moduleConfig)

	rt, err := starbase.New(&starbase.Config{
		Registry:   registry,
		ScriptPath: resourceFile,
		Print: func(thread *starlark.Thread, msg string) {
			output.WriteString(msg)
			output.WriteString("\n")
		},
	})
	if err != nil {
		return "", fmt.Errorf("runtime: %w", err)
	}

	if err := rt.Execute(context.Background(), string(data)); err != nil {
		return "", fmt.Errorf("execute %s: %w", resourceFile, err)
	}

	return output.String(), nil
}

func writeDockerfile(filename string) error {
	var b strings.Builder
	b.WriteString("FROM ghcr.io/vladimirvivien/kite-cloud:latest\n")
	b.WriteString(fmt.Sprintf("COPY %s /app/\n", genController))
	if genResource != "" {
		b.WriteString(fmt.Sprintf("COPY %s /app/\n", genResource))
	}
	b.WriteString(fmt.Sprintf("CMD [\"kite-cloud\", \"run\", \"/app/%s\"]\n", genController))
	return os.WriteFile(filename, []byte(b.String()), 0644)
}

// ============================================================================
// gen-webhook-artifacts command
// ============================================================================

var (
	whWebhook    string
	whName       string
	whImage      string
	whNamespace  string
	whRules      []string
	whOutput     string
	whDockerfile string
	whGenerator  string
)

var genWebhookArtifactsCmd = &cobra.Command{
	Use:   "gen-webhook-artifacts",
	Short: "Generate Kubernetes deployment artifacts for a webhook",
	Long: `Generate all deployment manifests for a starkite admission webhook.

Produces: Namespace, ServiceAccount, Deployment (with TLS volume),
Service (443→9443), TLS Secret placeholder, WebhookConfiguration.

Examples:
  kite-cloud kube gen-webhook-artifacts \
      --webhook webhook.star \
      --name myapp-webhook \
      --image myregistry/myapp-webhook:v1 \
      --namespace myapp-system \
      --rule "group=apps resource=deployments operations=CREATE,UPDATE" > deploy.yaml
`,
	RunE: runGenWebhookArtifacts,
}

func init() {
	genWebhookArtifactsCmd.Flags().StringVar(&whWebhook, "webhook", "webhook.star", "Path to webhook script")
	genWebhookArtifactsCmd.Flags().StringVar(&whName, "name", "", "Webhook name (required)")
	genWebhookArtifactsCmd.Flags().StringVar(&whImage, "image", "", "Container image (required)")
	genWebhookArtifactsCmd.Flags().StringVar(&whNamespace, "namespace", "default", "Namespace for deployment")
	genWebhookArtifactsCmd.Flags().StringArrayVar(&whRules, "rule", nil, "Admission rule: \"group=apps resource=deployments operations=CREATE,UPDATE\" (repeatable)")
	genWebhookArtifactsCmd.Flags().StringVar(&whOutput, "output", "yaml", "Output format: yaml or script")
	genWebhookArtifactsCmd.Flags().StringVar(&whDockerfile, "dockerfile", "", "Generate Dockerfile with given name")
	genWebhookArtifactsCmd.Flags().StringVar(&whGenerator, "generator", "", "Custom generator script")

	kubeCmd.AddCommand(genWebhookArtifactsCmd)
}

func runGenWebhookArtifacts(cmd *cobra.Command, args []string) error {
	if whImage == "" {
		return fmt.Errorf("--image is required")
	}
	if whName == "" {
		return fmt.Errorf("--name is required")
	}
	if whOutput != "yaml" && whOutput != "script" {
		return fmt.Errorf("--output must be 'yaml' or 'script', got %q", whOutput)
	}

	// Parse rules into JSON
	var parsedRules []map[string]interface{}
	for _, rule := range whRules {
		parsed, err := parseRule(rule)
		if err != nil {
			return fmt.Errorf("invalid --rule %q: %w", rule, err)
		}
		parsedRules = append(parsedRules, parsed)
	}
	rulesJSON, err := json.Marshal(parsedRules)
	if err != nil {
		return fmt.Errorf("failed to encode rules: %w", err)
	}

	// Determine generator script
	var scriptSource, scriptName string
	if whGenerator != "" {
		data, err := os.ReadFile(whGenerator)
		if err != nil {
			return fmt.Errorf("failed to read generator %q: %w", whGenerator, err)
		}
		scriptSource = string(data)
		scriptName = whGenerator
	} else {
		scriptSource = defaultWebhookGeneratorScript
		scriptName = "<builtin-webhook-generator>"
	}

	// Build variable store
	vars := varstore.New()
	vars.Set("webhook_script", whWebhook)
	vars.Set("webhook_name", whName)
	vars.Set("image", whImage)
	vars.Set("namespace", whNamespace)
	vars.Set("rules_json", string(rulesJSON))
	vars.Set("output_format", whOutput)

	// Create runtime
	moduleConfig := &starbase.ModuleConfig{VarStore: vars}
	registry := loader.NewCloudRegistry(moduleConfig)
	rt, err := starbase.New(&starbase.Config{
		Registry:   registry,
		VarStore:   vars,
		ScriptPath: scriptName,
	})
	if err != nil {
		return fmt.Errorf("failed to create runtime: %w", err)
	}

	if err := rt.Execute(context.Background(), scriptSource); err != nil {
		return fmt.Errorf("generator failed: %w", err)
	}

	// Dockerfile
	if whDockerfile != "" {
		var b strings.Builder
		b.WriteString("FROM ghcr.io/vladimirvivien/kite-cloud:latest\n")
		b.WriteString(fmt.Sprintf("COPY %s /app/\n", whWebhook))
		b.WriteString(fmt.Sprintf("CMD [\"kite-cloud\", \"run\", \"/app/%s\"]\n", whWebhook))
		if err := os.WriteFile(whDockerfile, []byte(b.String()), 0644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Dockerfile written to ./%s\n", whDockerfile)
	}

	return nil
}

// parseRule parses "group=apps version=v1 resource=deployments operations=CREATE,UPDATE"
func parseRule(rule string) (map[string]interface{}, error) {
	result := map[string]interface{}{}
	parts := strings.Fields(rule)
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("expected key=value, got %q", part)
		}
		key, val := kv[0], kv[1]
		switch key {
		case "group":
			result["apiGroups"] = []string{val}
		case "version":
			result["apiVersions"] = []string{val}
		case "resource":
			result["resources"] = []string{val}
		case "operations":
			result["operations"] = strings.Split(val, ",")
		default:
			return nil, fmt.Errorf("unknown key %q (valid: group, version, resource, operations)", key)
		}
	}
	if _, ok := result["apiVersions"]; !ok {
		result["apiVersions"] = []string{"*"}
	}
	if _, ok := result["resources"]; !ok {
		return nil, fmt.Errorf("'resource' is required")
	}
	return result, nil
}
