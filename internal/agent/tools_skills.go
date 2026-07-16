package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitea-agent-gateway/internal/llm"
	"gitea-agent-gateway/internal/sandbox"
)

// Skill represents a file-based skill loaded from SKILL.md
type Skill struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Tools       []SkillTool            `json:"tools"`
	FilePath    string                 `json:"file_path"`
	Loaded      bool                   `json:"loaded"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// SkillTool defines a tool provided by a skill
type SkillTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]SkillParam  `json:"parameters"`
	Required    []string               `json:"required"`
	Script      string                 `json:"script"`
}

// SkillParam defines a parameter for a skill tool
type SkillParam struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// SkillRegistry manages loaded skills
type SkillRegistry struct {
	skills     map[string]*Skill
	sandbox    *sandbox.Sandbox
	gatewayDir string
}

// NewSkillRegistry creates a new SkillRegistry
func NewSkillRegistry(sb *sandbox.Sandbox, gatewayDir string) *SkillRegistry {
	return &SkillRegistry{
		skills:     make(map[string]*Skill),
		sandbox:    sb,
		gatewayDir: gatewayDir,
	}
}

// ScanSkills scans for SKILL.md files in both gateway directory and workspace
func (r *SkillRegistry) ScanSkills() error {
	r.skills = make(map[string]*Skill)

	if r.gatewayDir != "" {
		if err := r.scanDir(r.gatewayDir); err != nil {
			return fmt.Errorf("scan gateway dir: %w", err)
		}
	}

	if err := r.scanDir(r.sandbox.WorkDir); err != nil {
		return fmt.Errorf("scan workspace: %w", err)
	}

	return nil
}

func (r *SkillRegistry) scanDir(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if strings.EqualFold(info.Name(), "SKILL.md") {
			if skill, err := parseSkillFile(path); err == nil {
				r.skills[skill.Name] = skill
			}
		}
		return nil
	})
}

// parseSkillFile parses a SKILL.md file
func parseSkillFile(path string) (*Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	skill := &Skill{
		FilePath: path,
		Tools:    []SkillTool{},
	}

	var currentTool *SkillTool
	inTool := false
	inScript := false

	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			skill.Name = strings.TrimPrefix(line, "# ")
			continue
		}

		if strings.HasPrefix(line, "## ") && !inTool {
			if strings.EqualFold(strings.TrimPrefix(line, "## "), "Description") {
				continue
			}
			if strings.EqualFold(strings.TrimPrefix(line, "## "), "Tools") {
				continue
			}
			continue
		}

		if skill.Description == "" && !strings.HasPrefix(line, "##") && !strings.HasPrefix(line, "-") {
			skill.Description = strings.TrimSpace(line)
			continue
		}

		if strings.HasPrefix(line, "- `") && strings.HasSuffix(line, "`") {
			if currentTool != nil {
				skill.Tools = append(skill.Tools, *currentTool)
			}
			toolName := strings.TrimPrefix(strings.TrimSuffix(line, "`"), "- `")
			currentTool = &SkillTool{
				Name:        toolName,
				Parameters:  make(map[string]SkillParam),
				Required:    []string{},
			}
			inTool = true
			inScript = false
			continue
		}

		if inTool && currentTool != nil {
			if strings.HasPrefix(line, "  - ") && !inScript {
				parts := strings.SplitN(strings.TrimPrefix(line, "  - "), ": ", 2)
				if len(parts) == 2 {
					key := parts[0]
					val := parts[1]
					if strings.HasPrefix(key, "param: ") {
						paramName := strings.TrimPrefix(key, "param: ")
						paramParts := strings.SplitN(val, " ", 2)
						paramType := paramParts[0]
						paramDesc := ""
						if len(paramParts) > 1 {
							paramDesc = paramParts[1]
						}
						currentTool.Parameters[paramName] = SkillParam{
							Type:        paramType,
							Description: paramDesc,
						}
						if strings.Contains(val, "(required)") {
							currentTool.Required = append(currentTool.Required, paramName)
						}
					} else if key == "description" {
						currentTool.Description = val
					} else if key == "script" {
						inScript = true
						currentTool.Script = val
						continue
					}
				}
			} else if inScript && strings.HasPrefix(line, "    ") {
				currentTool.Script += "\n" + strings.TrimPrefix(line, "    ")
			} else if line == "" && inScript {
				inScript = false
			}
		}
	}

	if currentTool != nil {
		skill.Tools = append(skill.Tools, *currentTool)
	}

	if skill.Name == "" {
		skill.Name = filepath.Base(filepath.Dir(path))
	}

	return skill, nil
}

// ListSkills returns all discovered skills
func (r *SkillRegistry) ListSkills() []*Skill {
	skills := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		skills = append(skills, s)
	}
	return skills
}

// GetSkill returns a skill by name
func (r *SkillRegistry) GetSkill(name string) (*Skill, bool) {
	s, ok := r.skills[name]
	return s, ok
}

// LoadSkill loads a skill's tools into the ToolRegistry.
// When allowScripts is false (Analyze), script-backed tools are skipped and the
// skill description is still marked loaded for progressive disclosure of metadata.
func (r *SkillRegistry) LoadSkill(name string, registry *ToolRegistry, allowScripts bool) error {
	skill, ok := r.skills[name]
	if !ok {
		return fmt.Errorf("skill not found: %s", name)
	}

	skippedScripts := 0
	for _, st := range skill.Tools {
		if strings.TrimSpace(st.Script) != "" && !allowScripts {
			skippedScripts++
			continue
		}
		if strings.TrimSpace(st.Script) == "" {
			// Non-script tools are not yet supported beyond registration metadata;
			// skip empty script entries to avoid registering no-op shell tools.
			continue
		}
		toolDef := skillToolToToolDef(st, r.sandbox)
		registry.Register(toolDef)
	}

	skill.Loaded = true
	if skippedScripts > 0 && !allowScripts {
		return fmt.Errorf("skill %q: %d script tool(s) blocked (analyze readonly); metadata loaded only", name, skippedScripts)
	}
	return nil
}

// skillToolToToolDef converts a SkillTool to a ToolDef
func skillToolToToolDef(st SkillTool, sb *sandbox.Sandbox) *ToolDef {
	params := llm.Parameters{
		Type:       "object",
		Properties: make(map[string]llm.Property),
		Required:   st.Required,
	}

	for name, p := range st.Parameters {
		params.Properties[name] = llm.Property{
			Type:        p.Type,
			Description: p.Description,
		}
	}

	return &ToolDef{
		Name:        st.Name,
		Description: st.Description,
		Parameters:  params,
		Fn: func(params map[string]interface{}) (string, error) {
			script := st.Script
			for name, val := range params {
				script = strings.ReplaceAll(script, "{{"+name+"}}", fmt.Sprintf("%v", val))
			}

			result := sb.ExecuteShell(script)
			output := result.Stdout
			if result.Stderr != "" {
				output += "\n" + result.Stderr
			}
			if result.Error != nil {
				output += fmt.Sprintf("\nError: %v", result.Error)
			}
			output += fmt.Sprintf("\nExit code: %d", result.ExitCode)
			return output, nil
		},
	}
}

// NewListSkillsTool creates a tool that lists available skills
func NewListSkillsTool(skillReg *SkillRegistry) *ToolDef {
	return &ToolDef{
		Name:        "list_skills",
		Description: "List available skills discovered from SKILL.md files in the workspace and gateway directory.",
		Parameters: llm.Parameters{
			Type: "object",
		},
		Fn: func(params map[string]interface{}) (string, error) {
			if err := skillReg.ScanSkills(); err != nil {
				return fmt.Sprintf("Error scanning skills: %v", err), nil
			}

			skills := skillReg.ListSkills()
			if len(skills) == 0 {
				return "No skills found. Skills are loaded from SKILL.md files in the workspace and gateway directory.", nil
			}

			var sb strings.Builder
			sb.WriteString("Available skills:\n")
			for _, s := range skills {
				status := "not loaded"
				if s.Loaded {
					status = "loaded"
				}
				sb.WriteString(fmt.Sprintf("- %s (%s): %s\n", s.Name, status, s.Description))
				if len(s.Tools) > 0 {
					sb.WriteString("  Tools:\n")
					for _, t := range s.Tools {
						sb.WriteString(fmt.Sprintf("    - %s: %s\n", t.Name, t.Description))
					}
				}
			}
			sb.WriteString("\nUse load_skill(name) to load a skill's tools.")
			return sb.String(), nil
		},
	}
}

// NewLoadSkillTool creates a tool that loads a specific skill.
// When allowScripts is false (Analyze), script tools are not registered.
func NewLoadSkillTool(skillReg *SkillRegistry, toolReg *ToolRegistry, allowScripts bool) *ToolDef {
	desc := "Load a skill's tools into the agent. After loading, the skill's tools become available for use."
	if !allowScripts {
		desc = "Load a skill for analysis. Script tools are blocked in analyze mode; only skill metadata/description is disclosed."
	}
	return &ToolDef{
		Name:        "load_skill",
		Description: desc,
		Parameters: llm.Parameters{
			Type: "object",
			Properties: map[string]llm.Property{
				"name": {
					Type:        "string",
					Description: "The name of the skill to load.",
				},
			},
			Required: []string{"name"},
		},
		Fn: func(params map[string]interface{}) (string, error) {
			name, _ := params["name"].(string)
			if name == "" {
				return "", fmt.Errorf("name is required")
			}

			err := skillReg.LoadSkill(name, toolReg, allowScripts)
			skill, _ := skillReg.GetSkill(name)

			var sb strings.Builder
			if err != nil {
				// Soft failure for analyze: still return description if skill was found
				if skill != nil && !allowScripts {
					sb.WriteString(fmt.Sprintf("Skill '%s' metadata loaded (scripts blocked).\n", name))
					if skill.Description != "" {
						sb.WriteString("Description: " + skill.Description + "\n")
					}
					sb.WriteString(err.Error())
					return sb.String(), nil
				}
				return fmt.Sprintf("Error loading skill: %v", err), nil
			}

			sb.WriteString(fmt.Sprintf("Skill '%s' loaded successfully.\n", name))
			if skill != nil {
				if skill.Description != "" {
					sb.WriteString("Description: " + skill.Description + "\n")
				}
				if allowScripts && len(skill.Tools) > 0 {
					sb.WriteString("Available tools:\n")
					for _, t := range skill.Tools {
						sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Name, t.Description))
					}
				}
			}
			return sb.String(), nil
		},
	}
}
