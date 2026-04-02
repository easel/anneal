package engine

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/easel/anneal/internal/manifest"
)

// dockerInspectFunc reads running container config. Injectable for testing.
// Returns nil if the container does not exist.
var dockerInspectFunc = dockerInspectReal

// DockerContainerState holds the relevant config of a running container.
type DockerContainerState struct {
	Image         string
	Ports         []string // "host:container" format
	Volumes       []string // "host:container" format
	Env           []string // "KEY=VALUE" format
	NetworkMode   string
	RestartPolicy string
	Args          []string // CMD/args
}

func dockerInspectReal(name string) (*DockerContainerState, error) {
	cmd := exec.Command("docker", "inspect", "--format", "{{json .}}", name)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil // Container doesn't exist
	}

	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("docker inspect: parse error: %w", err)
	}

	state := &DockerContainerState{}

	// Extract image
	if config, ok := raw["Config"].(map[string]any); ok {
		state.Image, _ = config["Image"].(string)
		if envList, ok := config["Env"].([]any); ok {
			for _, e := range envList {
				if s, ok := e.(string); ok {
					state.Env = append(state.Env, s)
				}
			}
		}
		if cmdList, ok := config["Cmd"].([]any); ok {
			for _, c := range cmdList {
				if s, ok := c.(string); ok {
					state.Args = append(state.Args, s)
				}
			}
		}
	}

	// Extract network mode
	if hostConfig, ok := raw["HostConfig"].(map[string]any); ok {
		state.NetworkMode, _ = hostConfig["NetworkMode"].(string)
		if rp, ok := hostConfig["RestartPolicy"].(map[string]any); ok {
			state.RestartPolicy, _ = rp["Name"].(string)
		}
		// Extract port bindings
		if bindings, ok := hostConfig["PortBindings"].(map[string]any); ok {
			for containerPort, hostPorts := range bindings {
				if hostList, ok := hostPorts.([]any); ok {
					for _, hp := range hostList {
						if hpMap, ok := hp.(map[string]any); ok {
							hostPort, _ := hpMap["HostPort"].(string)
							state.Ports = append(state.Ports, hostPort+":"+containerPort)
						}
					}
				}
			}
		}
		// Extract volume binds
		if binds, ok := hostConfig["Binds"].([]any); ok {
			for _, b := range binds {
				if s, ok := b.(string); ok {
					state.Volumes = append(state.Volumes, s)
				}
			}
		}
	}

	sort.Strings(state.Ports)
	sort.Strings(state.Volumes)
	sort.Strings(state.Env)
	sort.Strings(state.Args)

	return state, nil
}

// dockerContainerProvider manages Docker container lifecycle.
type dockerContainerProvider struct{}

func (dockerContainerProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	name, ok := resource.Spec["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("docker_container spec.name is required")
	}
	image, ok := resource.Spec["image"].(string)
	if !ok || image == "" {
		return nil, fmt.Errorf("docker_container spec.image is required")
	}

	// Build desired state from spec
	desired := &DockerContainerState{
		Image: image,
	}
	if ports, ok := resource.Spec["ports"].([]any); ok {
		for _, p := range ports {
			if s, ok := p.(string); ok {
				desired.Ports = append(desired.Ports, s)
			}
		}
	}
	if volumes, ok := resource.Spec["volumes"].([]any); ok {
		for _, v := range volumes {
			if s, ok := v.(string); ok {
				desired.Volumes = append(desired.Volumes, s)
			}
		}
	}
	if env, ok := resource.Spec["env"].([]any); ok {
		for _, e := range env {
			if s, ok := e.(string); ok {
				desired.Env = append(desired.Env, s)
			}
		}
	}
	if args, ok := resource.Spec["args"].([]any); ok {
		for _, a := range args {
			if s, ok := a.(string); ok {
				desired.Args = append(desired.Args, s)
			}
		}
	}
	desired.NetworkMode, _ = resource.Spec["network_mode"].(string)
	desired.RestartPolicy, _ = resource.Spec["restart_policy"].(string)

	sort.Strings(desired.Ports)
	sort.Strings(desired.Volumes)
	sort.Strings(desired.Env)
	sort.Strings(desired.Args)

	// Check current state
	current, err := dockerInspectFunc(name)
	if err != nil {
		return nil, fmt.Errorf("docker_container: inspecting %s: %w", name, err)
	}

	// Compare current to desired
	if current != nil && containerStatesMatch(current, desired) {
		return nil, nil // Already converged
	}

	var ops []string

	// Stop and remove existing container if it exists
	if current != nil {
		ops = append(ops, fmt.Sprintf("# recreate container %s (config drift)", name))
		ops = append(ops, fmt.Sprintf("stdlib_docker_stop %s", shellQuote(name)))
		ops = append(ops, fmt.Sprintf("stdlib_docker_rm %s", shellQuote(name)))
	} else {
		ops = append(ops, fmt.Sprintf("# create container %s", name))
	}

	// Build docker run arguments
	var runArgs []string
	runArgs = append(runArgs, "--name", shellQuote(name))

	if desired.RestartPolicy != "" {
		runArgs = append(runArgs, "--restart", shellQuote(desired.RestartPolicy))
	}
	if desired.NetworkMode != "" {
		runArgs = append(runArgs, "--network", shellQuote(desired.NetworkMode))
	}
	for _, p := range desired.Ports {
		runArgs = append(runArgs, "-p", shellQuote(p))
	}
	for _, v := range desired.Volumes {
		runArgs = append(runArgs, "-v", shellQuote(v))
	}
	for _, e := range desired.Env {
		runArgs = append(runArgs, "-e", shellQuote(e))
	}
	runArgs = append(runArgs, shellQuote(image))
	for _, a := range desired.Args {
		runArgs = append(runArgs, shellQuote(a))
	}

	ops = append(ops, fmt.Sprintf("stdlib_docker_run %s", strings.Join(runArgs, " ")))

	// Health check
	if healthURL, ok := resource.Spec["health_check_url"].(string); ok && healthURL != "" {
		ops = append(ops, fmt.Sprintf("stdlib_docker_health_check %s", shellQuote(healthURL)))
	}

	return ops, nil
}

// containerStatesMatch compares current and desired container states.
func containerStatesMatch(current, desired *DockerContainerState) bool {
	if current.Image != desired.Image {
		return false
	}
	if !slicesEqual(current.Ports, desired.Ports) {
		return false
	}
	if !slicesEqual(current.Volumes, desired.Volumes) {
		return false
	}
	if !slicesEqual(current.Args, desired.Args) {
		return false
	}
	// Compare env (only check desired keys exist in current with correct values)
	desiredEnvMap := envToMap(desired.Env)
	currentEnvMap := envToMap(current.Env)
	for k, v := range desiredEnvMap {
		if currentEnvMap[k] != v {
			return false
		}
	}
	if desired.NetworkMode != "" && current.NetworkMode != desired.NetworkMode {
		return false
	}
	if desired.RestartPolicy != "" && current.RestartPolicy != desired.RestartPolicy {
		return false
	}
	return true
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok {
			m[k] = v
		}
	}
	return m
}
