// Command api serves the KubeTRE HTTP API.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/api"
)

func main() {
	var (
		listen     = flag.String("listen", ":8080", "Address to serve HTTP on.")
		issuer     = flag.String("oidc-issuer", "", "OIDC issuer URL.")
		audience   = flag.String("oidc-audience", "", "Expected token audience (the API's client ID).")
		rolesClaim = flag.String("roles-claim", "roles", "Dot-separated claim path holding platform roles.")
		adminRole  = flag.String("admin-role", api.DefaultAdminRole, "Role that grants TRE administrator rights.")
		devAuth    = flag.Bool("insecure-dev-auth", false, "Trust X-Dev-User headers. Local development only.")
	)
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var auth api.Authenticator
	switch {
	case *devAuth:
		log.Warn("INSECURE: trusting X-Dev-User headers; never expose this server")
		auth = api.DevAuthenticator{}
	case *issuer != "" && *audience != "":
		a, err := api.NewOIDCAuthenticator(ctx, *issuer, *audience, *rolesClaim)
		if err != nil {
			log.Error("oidc setup failed", "error", err)
			os.Exit(1)
		}
		auth = a
	default:
		log.Error("set --oidc-issuer and --oidc-audience, or --insecure-dev-auth for local development")
		os.Exit(1)
	}

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = treV1.AddToScheme(scheme)
	c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		log.Error("kubernetes client", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              *listen,
		Handler:           (&api.Server{Client: c, Auth: auth, AdminRole: *adminRole, Log: log}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("serving", "addr", *listen)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server exited", "error", err)
		os.Exit(1)
	}
}
