package collector

import (
	"errors"
	"strings"
	"regexp"

	"github.com/fsouza/go-dockerclient"
)

// ErrNoNeedToMonitor is used to skip containers
// that shouldn't be monitored by collectd
var ErrNoNeedToMonitor = errors.New("container is not supposed to be monitored")

var imageNameRegex = regexp.MustCompile(`.*\/([^\/]*):.*`)

// MonitorDockerClient represents restricted interface for docker client
// that is used in monitor, docker.Client is a subset of this interface
type MonitorDockerClient interface {
	InspectContainer(id string) (*docker.Container, error)
	Stats(opts docker.StatsOptions) error
}

// Monitor is responsible for monitoring of a single container (task)
type Monitor struct {
	client   MonitorDockerClient
	id       string
	app      string
	task     string
	interval int
}

// NewMonitor creates new monitor with specified docker client,
// container id and stat updating interval
func NewMonitor(c MonitorDockerClient, id string, interval int) (*Monitor, error) {
	container, err := c.InspectContainer(id)
	if err != nil {
		return nil, err
	}

	app := sanitizeForGraphite(extractApp(container))
	if app == "" {
		return nil, ErrNoNeedToMonitor
	}

	task := sanitizeForGraphite(container.ID[:8])

	return &Monitor{
		client:   c,
		id:       container.ID,
		app:      app,
		task:     task,
		interval: interval,
	}, nil
}

func (m *Monitor) handle(ch chan<- Stats) error {
	in := make(chan *docker.Stats)

	go func() {
		i := 0
		for s := range in {
			if i%m.interval != 0 {
				i++
				continue
			}

			ch <- Stats{
				App:   m.app,
				Task:  m.task,
				Stats: *s,
			}

			i++
		}
	}()

	return m.client.Stats(docker.StatsOptions{
		ID:     m.id,
		Stats:  in,
		Stream: true,
	})
}

func extractApp(c *docker.Container) (app string) {
	app = extractEnv(c, "CHRONOS_JOB_NAME")
	if app != "" {
		return
	}
	app = extractEnv(c, "MARATHON_APP_ID")
	if app != "" {
		app = strings.TrimPrefix(app, "/")
		return
	}

	matches := imageNameRegex.FindStringSubmatch(c.Config.Image)
	if matches == nil || len(matches) < 1 {
		app = ""
		return
	}
	app = matches[0]

	return
}

func extractEnv(c *docker.Container, envVar string) string {
	envPrefix := envVar + "="
	for _, e := range c.Config.Env {
		if strings.HasPrefix(e, envPrefix) {
			return strings.TrimPrefix(e, envPrefix)
		}
	}

	return ""
}

func sanitizeForGraphite(s string) string {
	return strings.Replace(strings.Replace(s, ".", "_", -1), "/", "_", -1)
}
