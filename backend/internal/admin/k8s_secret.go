package admin

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/emersonpaula83/myplanner/backend/internal/config"
)

// K8sSecretWriter writes the rotated admin password into a Kubernetes
// Secret using the in-cluster service account credentials.
type K8sSecretWriter struct {
	clientset *kubernetes.Clientset
	name      string
	namespace string
	logger    *zap.Logger
}

func newK8sSecretWriter(cfg config.AdminSecretConfig, logger *zap.Logger) (*K8sSecretWriter, error) {
	k8sCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("getting in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		return nil, fmt.Errorf("creating k8s client: %w", err)
	}

	ns := cfg.SecretNamespace
	if ns == "" {
		nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			return nil, fmt.Errorf("detecting namespace: %w", err)
		}
		ns = string(nsBytes)
	}

	return &K8sSecretWriter{
		clientset: clientset,
		name:      cfg.SecretName,
		namespace: ns,
		logger:    logger,
	}, nil
}

// WritePassword creates or updates the target Secret with the newly
// rotated admin password and its expiry timestamp.
func (w *K8sSecretWriter) WritePassword(ctx context.Context, password string, expiresAt time.Time) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.name,
			Namespace: w.namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password":   []byte(password),
			"expires_at": []byte(expiresAt.Format(time.RFC3339)),
		},
	}

	secretsClient := w.clientset.CoreV1().Secrets(w.namespace)

	existing, err := secretsClient.Get(ctx, w.name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := secretsClient.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("creating k8s secret: %w", err)
			}
			w.logger.Info("k8s secret created", zap.String("name", w.name), zap.String("namespace", w.namespace))
			return nil
		}
		return fmt.Errorf("getting k8s secret: %w", err)
	}

	existing.Data = secret.Data
	if _, err := secretsClient.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating k8s secret: %w", err)
	}
	w.logger.Info("k8s secret updated", zap.String("name", w.name), zap.String("namespace", w.namespace))
	return nil
}

// StdoutSecretWriter is a development fallback that prints the rotated
// admin password to stdout instead of persisting it to a secret store.
type StdoutSecretWriter struct {
	logger *zap.Logger
}

// WritePassword prints the rotated password to stdout with a clear banner
// so it is easy to spot in local/dev logs.
func (w *StdoutSecretWriter) WritePassword(_ context.Context, password string, expiresAt time.Time) error {
	fmt.Printf("\n========================================\n")
	fmt.Printf("  ADMIN PASSWORD (dev mode)\n")
	fmt.Printf("  Password: %s\n", password)
	fmt.Printf("  Expires:  %s\n", expiresAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("========================================\n\n")
	w.logger.Info("admin password rotated (dev mode, printed to stdout)")
	return nil
}

// NewSecretWriter returns a K8sSecretWriter when running inside a
// Kubernetes cluster (detected via the KUBERNETES_SERVICE_HOST env var),
// falling back to a StdoutSecretWriter for local development or when
// in-cluster initialization fails.
func NewSecretWriter(cfg config.AdminSecretConfig, logger *zap.Logger) SecretWriter {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		writer, err := newK8sSecretWriter(cfg, logger)
		if err != nil {
			logger.Warn("failed to init K8s secret writer, falling back to stdout", zap.Error(err))
			return &StdoutSecretWriter{logger: logger}
		}
		return writer
	}
	return &StdoutSecretWriter{logger: logger}
}
