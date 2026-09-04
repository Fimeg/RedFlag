package services

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
)

// DockerSecretsService manages Docker secrets via Docker API
type DockerSecretsService struct {
	cli *client.Client
}

// NewDockerSecretsService creates a new Docker secrets service
func NewDockerSecretsService() (*DockerSecretsService, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Test connection
	ctx := context.Background()
	if _, err := cli.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to Docker daemon: %w", err)
	}

	return &DockerSecretsService{cli: cli}, nil
}

// CreateSecret creates a new Docker secret
func (s *DockerSecretsService) CreateSecret(name, value string) error {
	ctx := context.Background()

	// Check if secret already exists
	secrets, err := s.cli.SecretList(ctx, types.SecretListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	for _, secret := range secrets {
		if secret.Spec.Name == name {
			return fmt.Errorf("secret %s already exists", name)
		}
	}

	// Create the secret
	secretSpec := swarm.SecretSpec{
		Annotations: swarm.Annotations{
			Name: name,
			Labels: map[string]string{
				"created-by": "redflag-setup",
				"created-at": fmt.Sprintf("%d", 0), // Use current timestamp in real implementation
			},
		},
		Data: []byte(value),
	}

	if _, err := s.cli.SecretCreate(ctx, secretSpec); err != nil {
		return fmt.Errorf("failed to create secret %s: %w", name, err)
	}

	return nil
}

// DeleteSecret deletes a Docker secret
func (s *DockerSecretsService) DeleteSecret(name string) error {
	ctx := context.Background()

	// Find the secret
	secrets, err := s.cli.SecretList(ctx, types.SecretListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	var secretID string
	for _, secret := range secrets {
		if secret.Spec.Name == name {
			secretID = secret.ID
			break
		}
	}

	if secretID == "" {
		return fmt.Errorf("secret %s not found", name)
	}

	if err := s.cli.SecretRemove(ctx, secretID); err != nil {
		return fmt.Errorf("failed to remove secret %s: %w", name, err)
	}

	return nil
}

// Close closes the Docker client
func (s *DockerSecretsService) Close() error {
	if s.cli != nil {
		return s.cli.Close()
	}
	return nil
}

// IsDockerAvailable checks if Docker API is accessible
func IsDockerAvailable() bool {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	defer cli.Close()

	ctx := context.Background()
	_, err = cli.Ping(ctx)
	return err == nil
}
