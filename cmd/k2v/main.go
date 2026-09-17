// Command k2v migrates a Kubernetes Secret's keys into HashiCorp Vault,
// classified by config-driven rules. See docs/superpowers/specs for the
// full design.
package main

import (
	"flag"
	"fmt"
	"os"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/michaelact/k2v/internal/config"
	"github.com/michaelact/k2v/internal/vaultwriter"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] != "migrate" {
		fmt.Fprintln(os.Stderr, "usage: k2v migrate --namespace <ns> --secret <name> --config <path> [--apply] [--kubeconfig <path>]")
		fmt.Fprintln(os.Stderr, "   or: k2v migrate --all --config <path> [--apply] [--kubeconfig <path>]")
		return 1
	}

	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	namespace := fs.String("namespace", "", "Kubernetes namespace containing the Secret")
	secretName := fs.String("secret", "", "Name of the Kubernetes Secret to migrate")
	configPath := fs.String("config", "", "Path to the k2v config YAML file")
	apply := fs.Bool("apply", false, "Perform the writes (default is dry-run)")
	all := fs.Bool("all", false, "Discover every namespace/Secret pair matching the config's patterns, instead of a single --namespace/--secret")
	kubeconfig := fs.String("kubeconfig", "", "Path to kubeconfig (defaults to ~/.kube/config)")
	fs.Parse(args[1:])

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config is required")
		return 1
	}
	if *all && (*namespace != "" || *secretName != "") {
		fmt.Fprintln(os.Stderr, "error: --all can't be combined with --namespace/--secret")
		return 1
	}
	if !*all && (*namespace == "" || *secretName == "") {
		fmt.Fprintln(os.Stderr, "error: --namespace and --secret are required unless --all is set")
		return 1
	}

	k8sClient, err := buildK8sClient(*kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: building Kubernetes client: %v\n", err)
		return 1
	}

	var vaultWriter *vaultwriter.Writer
	if *apply {
		vaultClient, err := vaultapi.NewClient(vaultapi.DefaultConfig())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: building Vault client: %v\n", err)
			return 1
		}
		// vaultClient reads VAULT_ADDR/VAULT_TOKEN/VAULT_CACERT from the
		// environment automatically — no auth code needed here.
		vaultWriter = vaultwriter.New(vaultClient, mountFromConfig(*configPath))
	}

	return runMigrate(migrateArgs{
		namespace:  *namespace,
		secretName: *secretName,
		configPath: *configPath,
		apply:      *apply,
		all:        *all,
	}, k8sClient, vaultWriter, os.Stdout, os.Stderr)
}

func buildK8sClient(kubeconfigPath string) (kubernetes.Interface, error) {
	if kubeconfigPath == "" {
		if home, hErr := os.UserHomeDir(); hErr == nil {
			kubeconfigPath = home + "/.kube/config"
		}
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(restConfig)
}

// mountFromConfig re-reads the config just for the Vault mount name,
// since main() builds the Vault client before runMigrate loads the full
// config. Small duplication, avoids restructuring the flow for one field.
func mountFromConfig(path string) string {
	cfg, err := config.Load(path)
	if err != nil {
		return "default"
	}
	return cfg.Vault.Mount
}
