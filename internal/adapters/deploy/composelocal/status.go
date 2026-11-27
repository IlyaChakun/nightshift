package composelocal

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
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
	for line := range bytes.SplitSeq(out, []byte("\n")) {
		var c container
		if json.Unmarshal(bytes.TrimSpace(line), &c) == nil {
			list = append(list, c)
		}
	}
	return list
}

func (c container) label(key string) string {
	for kv := range strings.SplitSeq(c.Labels, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

// statusOf folds the containers of a project into one state. A container that exited with code
// 0 is a finished one-shot job (a migration, a seed) and does not make the stack unhealthy.
func statusOf(cs []container) deployapi.EnvStatus {
	if len(cs) == 0 {
		return deployapi.EnvStatus{State: deployapi.EnvAbsent}
	}
	st := deployapi.EnvStatus{State: deployapi.EnvHealthy}
	for _, c := range cs {
		if st.Revision == "" {
			st.Revision = deployapi.Revision(c.label(labelRevision))
			st.Previous = deployapi.Revision(c.label(labelPrevious))
		}
		health := strings.ToLower(c.Health + " " + c.Status)
		switch {
		case c.State == "exited" && strings.Contains(c.Status, "(0)"):
		case c.State != "running", strings.Contains(health, "unhealthy"):
			st.State = deployapi.EnvUnhealthy
		case strings.Contains(health, "starting") && st.State == deployapi.EnvHealthy:
			st.State = deployapi.EnvStarting
		}
	}
	return st
}
