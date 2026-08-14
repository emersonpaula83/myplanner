package admin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/config"
	"go.uber.org/zap"
)

func TestStdoutSecretWriter_WritePassword(t *testing.T) {
	w := &StdoutSecretWriter{logger: zap.NewNop()}
	err := w.WritePassword(context.Background(), "s3cr3t", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewSecretWriter_NoKubernetesEnv(t *testing.T) {
	orig, hadOrig := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	defer func() {
		if hadOrig {
			os.Setenv("KUBERNETES_SERVICE_HOST", orig)
		}
	}()

	writer := NewSecretWriter(config.AdminSecretConfig{}, zap.NewNop())
	if _, ok := writer.(*StdoutSecretWriter); !ok {
		t.Errorf("expected *StdoutSecretWriter, got %T", writer)
	}
}

func TestNewSecretWriter_KubernetesEnvSet_FallsBackWhenNotInCluster(t *testing.T) {
	orig, hadOrig := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	os.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	defer func() {
		if hadOrig {
			os.Setenv("KUBERNETES_SERVICE_HOST", orig)
		} else {
			os.Unsetenv("KUBERNETES_SERVICE_HOST")
		}
	}()

	// Outside a real cluster, rest.InClusterConfig() fails, so NewSecretWriter
	// should fall back to a StdoutSecretWriter rather than erroring out.
	writer := NewSecretWriter(config.AdminSecretConfig{}, zap.NewNop())
	if _, ok := writer.(*StdoutSecretWriter); !ok {
		t.Errorf("expected fallback to *StdoutSecretWriter, got %T", writer)
	}
}
