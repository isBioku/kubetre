// Command controller runs the KubeTRE workspace controller.
package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
	"github.com/isBioku/kubetre/internal/controller"
)

func main() {
	var metricsAddr, probeAddr string
	var leaderElect bool
	var vmPool string
	var vmSubnetLength int
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Address the metrics endpoint binds to; 0 disables it.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Address the health probe endpoint binds to.")
	flag.BoolVar(&leaderElect, "leader-elect", false, "Enable leader election so only one replica reconciles.")
	flag.StringVar(&vmPool, "vm-address-pool", "", "CIDR that workspace VM subnets are allocated from, for example 10.240.0.0/16. Empty disables VM workspaces.")
	flag.IntVar(&vmSubnetLength, "vm-subnet-length", 26, "Prefix length of each workspace's VM subnet.")
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	log := ctrl.Log.WithName("setup")

	scheme := runtime.NewScheme()
	must(clientgoscheme.AddToScheme(scheme))
	must(treV1.AddToScheme(scheme))

	cfg := ctrl.GetConfigOrDie()
	// A small managed control plane can be briefly unavailable. Wait for API discovery
	// rather than exiting into a crash loop, because controller setup needs it.
	if err := waitForAPIServer(cfg, 5*time.Minute); err != nil {
		log.Error(err, "Kubernetes API server unavailable")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         leaderElect,
		LeaderElectionID:       "workspace-controller.kubetre.io",
	})
	if err != nil {
		log.Error(err, "unable to create manager")
		os.Exit(1)
	}

	var pool *controller.VMPool
	if vmPool != "" {
		if pool, err = controller.ParseVMPool(vmPool, vmSubnetLength); err != nil {
			log.Error(err, "invalid VM address pool")
			os.Exit(1)
		}
		log.Info("VM workspaces enabled", "pool", vmPool, "subnets", pool.Size())
	}
	if err := (&controller.WorkspaceReconciler{
		Client: mgr.GetClient(), Scheme: mgr.GetScheme(), APIReader: mgr.GetAPIReader(), VMPool: pool,
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to set up workspace controller")
		os.Exit(1)
	}
	if err := (&controller.WorkspaceServiceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to set up workspace service controller")
		os.Exit(1)
	}
	must(mgr.AddHealthzCheck("healthz", healthz.Ping))
	must(mgr.AddReadyzCheck("readyz", healthz.Ping))

	log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "manager exited with error")
		os.Exit(1)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func waitForAPIServer(cfg *rest.Config, timeout time.Duration) error {
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for delay := time.Second; ; delay = min(delay*2, 30*time.Second) {
		_, err := dc.ServerGroups()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		ctrl.Log.WithName("setup").Info("waiting for the Kubernetes API server", "error", err.Error(), "retryIn", delay.String())
		time.Sleep(delay)
	}
}
