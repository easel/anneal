package engine

import (
	"strings"
	"testing"

	"github.com/easel/anneal/internal/manifest"
)

func withMockDocker(containers map[string]*DockerContainerState, fn func()) {
	orig := dockerInspectFunc
	dockerInspectFunc = func(name string) (*DockerContainerState, error) {
		return containers[name], nil
	}
	defer func() { dockerInspectFunc = orig }()
	fn()
}

func TestDockerContainerCreatesNew(t *testing.T) {
	withMockDocker(map[string]*DockerContainerState{}, func() {
		provider := dockerContainerProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "docker_container",
			Name: "my-nginx",
			Spec: map[string]any{
				"name":           "nginx-web",
				"image":          "nginx:latest",
				"ports":          []any{"80:80", "443:443"},
				"volumes":        []any{"/data/www:/usr/share/nginx/html:ro"},
				"env":            []any{"NGINX_HOST=example.com"},
				"restart_policy": "unless-stopped",
				"network_mode":   "bridge",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_docker_run") {
			t.Fatalf("expected docker run:\n%s", joined)
		}
		if !strings.Contains(joined, "'nginx:latest'") {
			t.Fatalf("expected image:\n%s", joined)
		}
		if !strings.Contains(joined, "-p '80:80'") {
			t.Fatalf("expected port mapping:\n%s", joined)
		}
		if !strings.Contains(joined, "-v '/data/www:/usr/share/nginx/html:ro'") {
			t.Fatalf("expected volume:\n%s", joined)
		}
		if !strings.Contains(joined, "-e 'NGINX_HOST=example.com'") {
			t.Fatalf("expected env:\n%s", joined)
		}
		if !strings.Contains(joined, "--restart 'unless-stopped'") {
			t.Fatalf("expected restart policy:\n%s", joined)
		}
		if !strings.Contains(joined, "# create container") {
			t.Fatalf("expected create comment:\n%s", joined)
		}
		// Should NOT have stop/rm since container doesn't exist yet
		if strings.Contains(joined, "stdlib_docker_stop") {
			t.Fatalf("should not stop non-existent container:\n%s", joined)
		}
	})
}

func TestDockerContainerConverged(t *testing.T) {
	withMockDocker(map[string]*DockerContainerState{
		"nginx-web": {
			Image:         "nginx:latest",
			Ports:         []string{"80:80"},
			Volumes:       []string{"/data:/data"},
			Env:           []string{"NGINX_HOST=example.com"},
			NetworkMode:   "bridge",
			RestartPolicy: "unless-stopped",
		},
	}, func() {
		provider := dockerContainerProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "docker_container",
			Name: "my-nginx",
			Spec: map[string]any{
				"name":           "nginx-web",
				"image":          "nginx:latest",
				"ports":          []any{"80:80"},
				"volumes":        []any{"/data:/data"},
				"env":            []any{"NGINX_HOST=example.com"},
				"restart_policy": "unless-stopped",
				"network_mode":   "bridge",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(ops) != 0 {
			t.Fatalf("expected no ops (converged), got: %v", ops)
		}
	})
}

func TestDockerContainerImageDrift(t *testing.T) {
	withMockDocker(map[string]*DockerContainerState{
		"nginx-web": {
			Image: "nginx:1.24",
			Ports: []string{"80:80"},
		},
	}, func() {
		provider := dockerContainerProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "docker_container",
			Name: "my-nginx",
			Spec: map[string]any{
				"name":  "nginx-web",
				"image": "nginx:1.25",
				"ports": []any{"80:80"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		// Should stop, remove, then run with new image
		if !strings.Contains(joined, "stdlib_docker_stop") {
			t.Fatalf("expected stop:\n%s", joined)
		}
		if !strings.Contains(joined, "stdlib_docker_rm") {
			t.Fatalf("expected rm:\n%s", joined)
		}
		if !strings.Contains(joined, "stdlib_docker_run") {
			t.Fatalf("expected run:\n%s", joined)
		}
		if !strings.Contains(joined, "config drift") {
			t.Fatalf("expected drift comment:\n%s", joined)
		}
	})
}

func TestDockerContainerPortDrift(t *testing.T) {
	withMockDocker(map[string]*DockerContainerState{
		"app": {
			Image: "myapp:latest",
			Ports: []string{"8080:80"},
		},
	}, func() {
		provider := dockerContainerProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "docker_container",
			Name: "app",
			Spec: map[string]any{
				"name":  "app",
				"image": "myapp:latest",
				"ports": []any{"9090:80"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(ops) == 0 {
			t.Fatal("expected ops for port drift")
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_docker_stop") {
			t.Fatalf("expected stop+rm+run cycle:\n%s", joined)
		}
	})
}

func TestDockerContainerWithHealthCheck(t *testing.T) {
	withMockDocker(map[string]*DockerContainerState{}, func() {
		provider := dockerContainerProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "docker_container",
			Name: "web",
			Spec: map[string]any{
				"name":             "web",
				"image":            "nginx:latest",
				"health_check_url": "http://localhost:80/health",
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "stdlib_docker_health_check") {
			t.Fatalf("expected health check:\n%s", joined)
		}
		if !strings.Contains(joined, "http://localhost:80/health") {
			t.Fatalf("expected health check URL:\n%s", joined)
		}
	})
}

func TestDockerContainerWithArgs(t *testing.T) {
	withMockDocker(map[string]*DockerContainerState{}, func() {
		provider := dockerContainerProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "docker_container",
			Name: "redis",
			Spec: map[string]any{
				"name":  "redis",
				"image": "redis:7",
				"args":  []any{"--maxmemory", "256mb"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		joined := strings.Join(ops, "\n")
		if !strings.Contains(joined, "'--maxmemory'") {
			t.Fatalf("expected args in run:\n%s", joined)
		}
		if !strings.Contains(joined, "'256mb'") {
			t.Fatalf("expected args in run:\n%s", joined)
		}
	})
}

func TestDockerContainerEnvDrift(t *testing.T) {
	withMockDocker(map[string]*DockerContainerState{
		"app": {
			Image: "myapp:latest",
			Env:   []string{"DB_HOST=old-db"},
		},
	}, func() {
		provider := dockerContainerProvider{}
		ops, err := provider.Plan(manifest.ResolvedResource{
			Kind: "docker_container",
			Name: "app",
			Spec: map[string]any{
				"name":  "app",
				"image": "myapp:latest",
				"env":   []any{"DB_HOST=new-db"},
			},
		})
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(ops) == 0 {
			t.Fatal("expected ops for env drift")
		}
	})
}

func TestDockerContainerValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    map[string]any
		wantErr string
	}{
		{"missing name", map[string]any{"image": "x"}, "name is required"},
		{"missing image", map[string]any{"name": "x"}, "image is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := dockerContainerProvider{}
			_, err := provider.Plan(manifest.ResolvedResource{
				Kind: "docker_container", Name: "test", Spec: tt.spec,
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestContainerStatesMatchIgnoresExtraEnv(t *testing.T) {
	// Container may have extra env vars from Docker itself — only declared ones matter
	current := &DockerContainerState{
		Image: "myapp:latest",
		Env:   []string{"DB_HOST=db", "PATH=/usr/bin", "HOME=/root"},
	}
	desired := &DockerContainerState{
		Image: "myapp:latest",
		Env:   []string{"DB_HOST=db"},
	}
	if !containerStatesMatch(current, desired) {
		t.Fatal("should match (extra env in current is ok)")
	}
}
