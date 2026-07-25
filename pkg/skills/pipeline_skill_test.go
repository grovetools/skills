package skills

import (
	"bytes"
	"fmt"
	"path"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type featurePipelineSpec struct {
	SchemaVersion int                    `yaml:"schema_version"`
	Name          string                 `yaml:"name"`
	Stages        []featurePipelineStage `yaml:"stages"`
}

type featurePipelineStage struct {
	ID          string            `yaml:"id"`
	After       []string          `yaml:"after,omitempty"`
	Execution   string            `yaml:"execution,omitempty"`
	Skill       string            `yaml:"skill,omitempty"`
	Produces    map[string]string `yaml:"produces,omitempty"`
	MaxAttempts int               `yaml:"max_attempts,omitempty"`
	Gate        string            `yaml:"gate,omitempty"`
	Optional    bool              `yaml:"optional,omitempty"`
}

func TestBuiltinFeaturePipelinePackage(t *testing.T) {
	loaded, err := LoadSkillFromSource("grove-feature-pipeline", SkillSource{
		Type:    SourceTypeBuiltin,
		RelPath: "grove-feature-pipeline",
	})
	if err != nil {
		t.Fatalf("load embedded skill: %v", err)
	}

	for _, file := range []string{
		"SKILL.md",
		"references/stage-contract.md",
		"references/recovery-policy.md",
		"references/flavors/agent-verified-feature.md",
		"references/flavors/quick-fix.md",
		"assets/pipelines/agent-verified-feature.yml",
		"assets/pipelines/quick-fix.yml",
		"assets/prompts/explore.md",
		"assets/prompts/verify-spec.md",
		"assets/prompts/review-code.md",
		"assets/prompts/final-audit.md",
	} {
		if _, ok := loaded.Files[file]; !ok {
			t.Errorf("embedded package missing %s", file)
		}
	}

	root := loaded.Files["SKILL.md"]
	if err := ValidateSkillContent(root, "grove-feature-pipeline"); err != nil {
		t.Fatalf("invalid root SKILL.md: %v", err)
	}
	for _, required := range []string{
		"Load **exactly one** matching flavor reference",
		"machine-consumed executable input to `flow_pipeline`, never advisory prose",
		"`flow_subjob join` before reading",
	} {
		if !bytes.Contains(root, []byte(required)) {
			t.Errorf("root SKILL.md missing contract %q", required)
		}
	}
}

func TestFeaturePipelineSpecsAreStrictValidDAGs(t *testing.T) {
	for _, name := range []string{"agent-verified-feature", "quick-fix"} {
		t.Run(name, func(t *testing.T) {
			body, err := embeddedSkillsFS.ReadFile("data/skills/grove-feature-pipeline/assets/pipelines/" + name + ".yml")
			if err != nil {
				t.Fatal(err)
			}

			var spec featurePipelineSpec
			dec := yaml.NewDecoder(bytes.NewReader(body))
			dec.KnownFields(true)
			if err := dec.Decode(&spec); err != nil {
				t.Fatalf("strict YAML decode: %v", err)
			}
			if spec.SchemaVersion != 1 || spec.Name != name || len(spec.Stages) == 0 {
				t.Fatalf("invalid header: version=%d name=%q stages=%d", spec.SchemaVersion, spec.Name, len(spec.Stages))
			}
			validateFeaturePipelineDAG(t, spec)
		})
	}
}

func validateFeaturePipelineDAG(t *testing.T, spec featurePipelineSpec) {
	t.Helper()
	stages := make(map[string]featurePipelineStage, len(spec.Stages))
	for _, stage := range spec.Stages {
		if stage.ID == "" {
			t.Fatal("stage has empty id")
		}
		if _, exists := stages[stage.ID]; exists {
			t.Fatalf("duplicate stage id %q", stage.ID)
		}
		if stage.Execution != "" && stage.Execution != "fanout" && stage.Execution != "adaptive" {
			t.Fatalf("stage %q has invalid execution %q", stage.ID, stage.Execution)
		}
		if stage.Gate != "" && stage.Gate != "human" {
			t.Fatalf("stage %q has invalid gate %q", stage.ID, stage.Gate)
		}
		if stage.Gate == "" && stage.MaxAttempts < 1 {
			t.Fatalf("executable stage %q must have bounded max_attempts", stage.ID)
		}
		for logical, artifact := range stage.Produces {
			if logical == "" || artifact == "" || path.IsAbs(artifact) || artifact == ".." || strings.HasPrefix(artifact, "../") {
				t.Fatalf("stage %q has unsafe artifact %q=%q", stage.ID, logical, artifact)
			}
		}
		stages[stage.ID] = stage
	}

	for _, stage := range spec.Stages {
		for _, dependency := range stage.After {
			if _, ok := stages[dependency]; !ok {
				t.Fatalf("stage %q depends on unknown stage %q", stage.ID, dependency)
			}
		}
	}

	state := make(map[string]uint8, len(stages))
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("cycle at %s", id)
		case 2:
			return nil
		}
		state[id] = 1
		for _, dependency := range stages[id].After {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range stages {
		if err := visit(id); err != nil {
			t.Fatalf("pipeline is not a DAG: %v", err)
		}
	}

	implement, ok := stages["implement"]
	if !ok || implement.Gate != "" {
		t.Fatal("pipeline must contain executable implement mutation stage")
	}
	if !hasDependentStage(stages, "implement", map[string]bool{"tests": true, "validate": true}) {
		t.Fatal("implement mutation has no validation successor")
	}
}

func hasDependentStage(stages map[string]featurePipelineStage, dependency string, allowed map[string]bool) bool {
	for id, stage := range stages {
		if !allowed[id] {
			continue
		}
		for _, after := range stage.After {
			if after == dependency {
				return true
			}
		}
	}
	return false
}
