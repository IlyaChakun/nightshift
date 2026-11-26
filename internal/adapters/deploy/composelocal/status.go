package composelocal

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// container is the part of `docker ps` and `docker compose ps` output the adapter reads. Both
// print a JSON array or one object per line, depending on the version.
type container struct {
	State      string `json:"State"`
	Status     string `json:"Status"`
	Health     string `json:"Health"`
	Labels     string `json:"Labels"`
	Publishers []struct {
		PublishedPort int `json:"PublishedPort"`
	} `json:"Publishers"`
}

func parsePS(out []byte) []container {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil
	}
	var list []container
	if out[0] == '[' {
		if json.Unmarshal(out, &list) == nil {
			return list
		}
		return nil
	}
	for _, line := range bytes.Split(out, []byte("\n")) {
		var c container
		if json.Unmarshal(bytes.TrimSpace(line), &c) == nil {
			list = append(list, c)
		}
	}
	return list
}

func (c container) label(key string) string {
	for _, kv := range strings.Split(c.Labels, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

// statusOf folds the containers of a project into one state. A container that exited with code
// 0 is a finished one-shot job (a migration, a seed) and does not make the stack unhealthy.
func statusOf(cs []container) integration.EnvStatus {
	if len(cs) == 0 {
		return integration.EnvStatus{State: integration.EnvAbsent}
	}
	st := integration.EnvStatus{State: integration.EnvHealthy}
	for _, c := range cs {
		if st.Revision == "" {
			st.Revision = integration.Revision(c.label(labelRevision))
			st.Previous = integration.Revision(c.label(labelPrevious))
		}
		health := strings.ToLower(c.Health + " " + c.Status)
		switch {
		case c.State == "exited" && strings.Contains(c.Status, "(0)"):
		case c.State != "running", strings.Contains(health, "unhealthy"):
			st.State = integration.EnvUnhealthy
		case strings.Contains(health, "starting") && st.State == integration.EnvHealthy:
			st.State = integration.EnvStarting
		}
	}
	return st
}
