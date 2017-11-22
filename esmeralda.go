package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/trace"
	"syscall"

	"chuanyun.io/esmeralda/config"
	"chuanyun.io/esmeralda/util"

	"golang.org/x/sync/errgroup"

	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	commit     = "2000.01.01.release"
	buildstamp = "2000-01-01T00:00:00+0800"

	exitChan = make(chan int)
)

type EsmeraldaServer interface {
	Start()
	Shutdown(code int, reason string)
}

type EsmeraldaServerImpl struct {
	context       context.Context
	shutdownFn    context.CancelFunc
	childRoutines *errgroup.Group
}

func NewEsmeraldaServer() EsmeraldaServer {
	rootCtx, shutdownFn := context.WithCancel(context.Background())
	childRoutines, childCtx := errgroup.WithContext(rootCtx)

	return &EsmeraldaServerImpl{
		context:       childCtx,
		shutdownFn:    shutdownFn,
		childRoutines: childRoutines,
	}
}

func (this *EsmeraldaServerImpl) Start() {

	config.Initialize(viper.GetString("config"))

	logrus.Info(util.Message("EsmeraldaServer Start"))

	go listenToSystemSignals(this)

	if viper.GetBool("pprof") {
		fmt.Println("Hello, pprof!")

		runtime.SetBlockProfileRate(1)
		go func() {
			http.ListenAndServe(fmt.Sprintf("localhost:%d", viper.GetInt("pprof.port")), nil)
		}()

		f, err := os.Create("trace.out")
		if err != nil {
			panic(err)
		}
		defer f.Close()

		err = trace.Start(f)
		if err != nil {
			panic(err)
		}
		defer trace.Stop()
	}

	go exporter()
}

func (this *EsmeraldaServerImpl) Shutdown(code int, reason string) {

	// g.log.Info("Shutdown started", "code", code, "reason", reason)

	// err := g.httpServer.Shutdown(g.context)
	// if err != nil {
	// 	g.log.Error("Failed to shutdown server", "error", err)
	// }

	// g.shutdownFn()
	// err = g.childRoutines.Wait()

	// g.log.Info("Shutdown completed", "reason", err)
	// log.Close()
	// os.Exit(code)

}

func main() {

	runtime.GOMAXPROCS(runtime.NumCPU())

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Long:  `Print the release software version info`,
		Run: func(cmd *cobra.Command, args []string) {

			fmt.Println(filepath.Base(os.Args[0]))
			fmt.Println("commit: " + commit + ", build: " + buildstamp)
			fmt.Println("Copyright (c) 2017, chuanyun.io. All rights reserved.")

			os.Exit(0)
		},
	}

	var rootCommand = &cobra.Command{
		Use:  filepath.Base(os.Args[0]),
		Long: `Esmeralda is a Full Stack Distributed Tracing Monitoring System Server`,
		Run: func(cmd *cobra.Command, args []string) {
			server := NewEsmeraldaServer()
			server.Start()
		},
	}

	flags := rootCommand.Flags()

	flags.String("config", "/etc/chuanyun/esmeralda.toml", "config file path")
	flags.Bool("pprof", false, "Turn on pprof profiling")
	flags.Int("pprof.port", 11011, "Define custom port for profiling")

	viper.BindPFlag("config", flags.Lookup("config"))
	viper.BindPFlag("pprof", flags.Lookup("pprof"))
	viper.BindPFlag("pprof.port", flags.Lookup("pprof.port"))

	rootCommand.AddCommand(versionCmd)
	rootCommand.Execute()
}

func Index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "Welcome!\n")
}

func Hello(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	fmt.Fprintf(w, "hello, %s!\n", ps.ByName("name"))
}

func exporter() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
		<html>
			<head><title>Metrics Exporter</title></head>
			<body>
				<h1>Metrics Exporter</h1>
				<p><a href="/metrics">Metrics</a></p>
			</body>
		</html>`))
	})
	http.Handle("/metrics", promhttp.Handler())
	// logrus.Fatal(http.ListenAndServe(":"+config.Config.Prometheus.Port, nil))
}

func listenToSystemSignals(server EsmeraldaServer) {
	signalChan := make(chan os.Signal, 1)
	ignoreChan := make(chan os.Signal, 1)
	code := 0

	signal.Notify(ignoreChan, syscall.SIGHUP)
	signal.Notify(signalChan, os.Interrupt, os.Kill, syscall.SIGTERM)

	select {
	case sig := <-signalChan:
		// Stops trace if profiling has been enabled
		trace.Stop()
		server.Shutdown(0, util.Message(fmt.Sprintf("system signal: %s", sig)))
	case code = <-exitChan:
		server.Shutdown(code, util.Message("startup error"))
	}
}
