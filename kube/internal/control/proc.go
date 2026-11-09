package control

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/spf13/pflag"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	cliflag "k8s.io/component-base/cli/flag"
	apiapp "k8s.io/kubernetes/cmd/kube-apiserver/app"
	apioptions "k8s.io/kubernetes/cmd/kube-apiserver/app/options"
	cmapp "k8s.io/kubernetes/cmd/kube-controller-manager/app"
	cmoptions "k8s.io/kubernetes/cmd/kube-controller-manager/app/options"
	schedapp "k8s.io/kubernetes/cmd/kube-scheduler/app"
	schedoptions "k8s.io/kubernetes/cmd/kube-scheduler/app/options"
)

func runAPIServer(ctx context.Context, args []string) error {
	opts := apioptions.NewServerRunOptions()
	if err := parseFlags("kube-apiserver", opts.Flags(), args); err != nil {
		return err
	}
	if err := opts.GenericServerRunOptions.ComponentGlobalsRegistry.Set(); err != nil {
		return err
	}
	completed, err := opts.Complete(ctx)
	if err != nil {
		return err
	}
	if errs := completed.Validate(); len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}
	return apiapp.Run(ctx, completed)
}

func runControllerManager(ctx context.Context, args []string) error {
	opts, err := cmoptions.NewKubeControllerManagerOptions()
	if err != nil {
		return err
	}
	known := cmapp.KnownControllers()
	disabled := cmapp.ControllersDisabledByDefault()
	aliases := cmapp.ControllerAliases()
	if err := parseFlags("kube-controller-manager", opts.Flags(known, disabled, aliases), args); err != nil {
		return err
	}
	if err := opts.ComponentGlobalsRegistry.Set(); err != nil {
		return err
	}
	cfg, err := opts.Config(ctx, known, disabled, aliases)
	if err != nil {
		return err
	}
	return cmapp.Run(ctx, cfg.Complete())
}

func runScheduler(ctx context.Context, args []string) error {
	opts := schedoptions.NewOptions()
	if err := parseFlags("kube-scheduler", *opts.Flags, args); err != nil {
		return err
	}
	completed, sched, err := schedapp.Setup(ctx, opts)
	if err != nil {
		return err
	}
	return schedapp.Run(ctx, completed, sched)
}

func parseFlags(name string, sets cliflag.NamedFlagSets, args []string) error {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	var buf bytes.Buffer
	fs.SetOutput(&buf)
	for _, set := range sets.FlagSets {
		fs.AddFlagSet(set)
	}
	if err := fs.Parse(args); err != nil {
		return flagParseError(name, err, &buf)
	}
	return nil
}

func flagParseError(name string, err error, buf io.Reader) error {
	detail, _ := io.ReadAll(buf)
	if len(detail) == 0 {
		return fmt.Errorf("%s: %w", name, err)
	}
	return fmt.Errorf("%s: %w: %s", name, err, bytes.TrimSpace(detail))
}

func report(errCh chan<- error, name string, err error) {
	if err == nil {
		return
	}
	select {
	case errCh <- fmt.Errorf("%s: %w", name, err):
	default:
	}
}
