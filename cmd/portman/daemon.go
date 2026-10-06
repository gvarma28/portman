package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gvarma28/portman/internal/ports"
	"github.com/gvarma28/portman/internal/sshpool"
	yaml "go.yaml.in/yaml/v3"
)

func main() {
	configPath := flag.String("config", defaultConfigPath(), "path to the config file")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := sshpool.New()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	pm := ports.NewPortMap(pool.Get)

	parseConfig(*configPath, pm)

	<-ctx.Done()
	log.Println("shutting down gracefully")
}

// defaultConfigPath returns ~/.config/portman/config.yaml, or config.yaml in
// the working directory if the home directory can't be determined.
func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(home, ".config", "portman", "config.yaml")
}

func parseConfig(path string, portMap *ports.PortMap) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	var cfg ports.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatal(err)
	}

	for _, h := range cfg.Hosts {
		for _, f := range h.Forwards {
			err := portMap.Add(f.Local, f.Remote, h.Host)
			if err != nil {
				log.Printf("error while adding portMap: %v", err)

			}
		}
	}
}
