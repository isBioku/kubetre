// Command gateway runs the KubeTRE access gateway broker in front of Apache Guacamole.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/gateway"
)

func main() {
	var (
		listen      = flag.String("listen", ":8080", "Address to serve HTTP on.")
		externalURL = flag.String("external-url", "", "Public URL of the gateway, for example https://gateway.example.org.")
		issuer      = flag.String("oidc-issuer", "", "OIDC issuer URL.")
		clientID    = flag.String("oidc-client-id", "", "OIDC client ID of the gateway's app registration.")
		guacPath    = flag.String("guacamole-path", "/guacamole/", "Path where Guacamole is served on the same host.")
		keysSecret  = flag.String("keys-secret", "kubetre-gateway-keys", "Secret holding the Guacamole JSON key and the session key; created if missing.")
	)
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	fail := func(msg string, err error) {
		log.Error(msg, "error", err)
		os.Exit(1)
	}

	ext, err := url.Parse(*externalURL)
	if err != nil || ext.Scheme != "https" || ext.Host == "" {
		fail("--external-url must be an https URL", err)
	}
	if *issuer == "" || *clientID == "" {
		fail("--oidc-issuer and --oidc-client-id are required", nil)
	}
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		fail("POD_NAMESPACE must be set", nil)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = treV1.AddToScheme(scheme)
	c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		fail("kubernetes client", err)
	}

	guacHex, sessionKey, err := ensureKeys(ctx, c, namespace, *keysSecret)
	if err != nil {
		fail("gateway keys", err)
	}
	guacKey, err := gateway.ParseGuacKey(guacHex)
	if err != nil {
		fail("gateway keys", err)
	}
	sealer, err := gateway.NewSealer(sessionKey)
	if err != nil {
		fail("gateway keys", err)
	}
	login, err := gateway.NewOIDCLogin(ctx, *issuer, *clientID, os.Getenv("OIDC_CLIENT_SECRET"), ext.String()+"/callback")
	if err != nil {
		fail("oidc", err)
	}

	srv := &http.Server{
		Addr: *listen,
		Handler: (&gateway.Server{
			Resolver: &gateway.Resolver{Client: c}, Login: login, Sealer: sealer, GuacKey: guacKey,
			ExternalURL: ext, GuacamolePath: *guacPath, Log: log,
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("serving", "addr", *listen, "external", ext.String())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fail("server exited", err)
	}
}

// ensureKeys reads the gateway's keys, generating them on first start. Guacamole reads the
// JSON key from the same Secret, so it is never written to Git or Terraform state.
func ensureKeys(ctx context.Context, c client.Client, namespace, name string) (string, []byte, error) {
	for attempt := 0; attempt < 3; attempt++ {
		sec := &corev1.Secret{}
		err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sec)
		if err == nil {
			return string(sec.Data["json-secret-key"]), sec.Data["session-key"], nil
		}
		if !apierrors.IsNotFound(err) {
			return "", nil, err
		}
		guac := make([]byte, 16)
		session := make([]byte, 32)
		if _, err := rand.Read(guac); err != nil {
			return "", nil, err
		}
		if _, err := rand.Read(session); err != nil {
			return "", nil, err
		}
		sec = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"app.kubernetes.io/part-of": "kubetre-gateway"}},
			Data:       map[string][]byte{"json-secret-key": []byte(hex.EncodeToString(guac)), "session-key": session},
		}
		if err := c.Create(ctx, sec); err != nil && !apierrors.IsAlreadyExists(err) {
			return "", nil, err
		}
		// Re-read: another replica may have won the race.
	}
	return "", nil, errors.New("could not create or read the gateway keys secret")
}
