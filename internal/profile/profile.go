package profile

import (
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type Document struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

type Metadata struct {
	Name string `yaml:"name"`
}

type Spec struct {
	Model                 string          `yaml:"model"`
	Revision              string          `yaml:"revision"`
	EngineImage           string          `yaml:"engineImage"`
	Hardware              Hardware        `yaml:"hardware"`
	Engine                Engine          `yaml:"engine"`
	SamplingPeriodSeconds int64           `yaml:"samplingPeriodSeconds"`
	Capacity              []CapacityPoint `yaml:"capacity"`
	SourceEvidence        string          `yaml:"sourceEvidence"`
}

type Hardware struct {
	Vendor    string `yaml:"vendor"`
	Product   string `yaml:"product"`
	Count     int    `yaml:"count"`
	MemoryGiB int    `yaml:"memoryGiB"`
}

type Engine struct {
	TensorParallel       int     `yaml:"tensorParallel"`
	MaxModelLen          int     `yaml:"maxModelLen"`
	GPUMemoryUtilization float64 `yaml:"gpuMemoryUtilization"`
}

type CapacityPoint struct {
	InputTokens  int     `yaml:"inputTokens"`
	OutputTokens int     `yaml:"outputTokens"`
	SafeQPS      float64 `yaml:"safeQPS"`
	TTFTP95Ms    float64 `yaml:"ttftP95Ms"`
}

func Load(path string) (Document, error) {
	file, err := os.Open(path)
	if err != nil {
		return Document{}, fmt.Errorf("open profile: %w", err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("decode profile: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Document{}, errors.New("profile must contain exactly one YAML document")
		}
		return Document{}, fmt.Errorf("decode trailing profile content: %w", err)
	}
	if err := document.Validate(); err != nil {
		return Document{}, err
	}
	return document, nil
}

func (document Document) Validate() error {
	if document.APIVersion != "inference.bupt.dev/v1alpha1" {
		return fmt.Errorf("unsupported profile apiVersion %q", document.APIVersion)
	}
	if document.Kind != "ModelBackendProfile" {
		return fmt.Errorf("unsupported profile kind %q", document.Kind)
	}
	if document.Metadata.Name == "" || document.Spec.Model == "" || document.Spec.Revision == "" {
		return errors.New("profile name, model, and revision are required")
	}
	if document.Spec.EngineImage == "" {
		return errors.New("profile engineImage is required")
	}
	if document.Spec.Hardware.Vendor == "" || document.Spec.Hardware.Product == "" ||
		document.Spec.Hardware.Count <= 0 || document.Spec.Hardware.MemoryGiB <= 0 {
		return errors.New("profile hardware fields must be populated and positive")
	}
	if document.Spec.Engine.TensorParallel <= 0 || document.Spec.Engine.MaxModelLen <= 0 ||
		document.Spec.Engine.GPUMemoryUtilization <= 0 || document.Spec.Engine.GPUMemoryUtilization > 1 {
		return errors.New("profile engine fields are invalid")
	}
	if document.Spec.SamplingPeriodSeconds <= 0 {
		return errors.New("profile samplingPeriodSeconds must be positive")
	}
	if len(document.Spec.Capacity) == 0 {
		return errors.New("profile requires at least one capacity point")
	}
	seen := make(map[[2]int]struct{}, len(document.Spec.Capacity))
	for _, point := range document.Spec.Capacity {
		if point.InputTokens <= 0 || point.OutputTokens <= 0 || point.SafeQPS <= 0 || point.TTFTP95Ms <= 0 {
			return errors.New("profile capacity values must be positive")
		}
		bucket := [2]int{point.InputTokens, point.OutputTokens}
		if _, exists := seen[bucket]; exists {
			return fmt.Errorf("duplicate capacity bucket %d/%d", point.InputTokens, point.OutputTokens)
		}
		seen[bucket] = struct{}{}
	}
	if document.Spec.SourceEvidence == "" {
		return errors.New("profile sourceEvidence is required")
	}
	return nil
}
