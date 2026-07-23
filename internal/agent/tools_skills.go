package agent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jeeinn/matea/internal/llm"
	"github.com/jeeinn/matea/internal/sandbox"
)

// skillScanDirs lists the well-known directories (relative to a scan root) to
// look for skill files. Order determines priority when the same skill name
// appears in multiple locations.
var skillScanDirs = []string{
	"skills",
	".agents/skills",
}

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// Skill represents a file-based skill loaded from a SKILL.md (or *.md) file.
type Skill struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Tools       []SkillTool            `json:"tools"`
	FilePath    string                 `json:"file_path"`
	Body        string                 `json:"body,omitempty"` // Markdown body after frontmatter
	Loaded      bool                   `json:"loaded"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// SkillTool defines a tool provided by a skill.
type SkillTool struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Parameters  []SkillParam `json:"parameters"`
	Required    []string     `json:"required"`
	Script      string       `json:"script"`
}

// SkillParam defines a parameter for a skill tool.
type SkillParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// skillFrontmatter is the YAML structure inside the --- block.
type skillFrontmatter struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Tools       []skillFMTool `yaml:"tools"`
}

type skillFMTool struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Script      string         `yaml:"script"`
	Parameters  []skillFMParam `yaml:"parameters"`
}

type skillFMParam struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

// ---------------------------------------------------------------------------
// SkillRegistry
// ---------------------------------------------------------------------------

// SkillRegistry manages discovered and loaded skills.
type SkillRegistry struct {
	skills     map[string]*Skill
	sandbox    *sandbox.Sandbox
	gatewayDir string
}

// NewSkillRegistry creates a new SkillRegistry.
func NewSkillRegistry(sb *sandbox.Sandbox, gatewayDir string) *SkillRegistry {
	return &SkillRegistry{
		skills:     make(map[string]*Skill),
		sandbox:    sb,
		gatewayDir: gatewayDir,
	}
}

// ScanSkills scans for skill files in both gateway directory and workspace.
// It looks in well-known roots: skills/, .agents/skills/, and standalone SKILL.md.
func (r *SkillRegistry) ScanSkills() error {
	r.skills = make(map[string]*Skill)

	if r.gatewayDir != "" {
		if err := r.scanRoot(r.gatewayDir); err != nil {
			return fmt.Errorf("scan gateway dir: %w", err)
		}
	}

	if r.sandbox.WorkDir != "" {
		if err := r.scanRoot(r.sandbox.WorkDir); err != nil {
			return fmt.Errorf("scan workspace: %w", err)
		}
	}

	return nil
}

// scanRoot scans a single root directory for skill files.
func (r *SkillRegistry) scanRoot(root string) error {
	// 1) Scan well-known skill directories (skills/, .agents/skills/)
	for _, rel := range skillScanDirs {
		dir := filepath.Join(root, rel)
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			if err := r.scanSkillDir(dir); err != nil {
				return err
			}
		}
	}

	// 2) Legacy: standalone SKILL.md at the root itself
	legacy := filepath.Join(root, "SKILL.md")
	if fi, err := os.Stat(legacy); err == nil && !fi.IsDir() {
		if skill, err := parseSkillFile(legacy); err == nil && skill.Name != "" {
			if _, exists := r.skills[skill.Name]; !exists {
				r.skills[skill.Name] = skill
			}
		}
	}

	return nil
}

// scanSkillDir loads all *.md files from a dedicated skill directory.
// Each file is treated as one skill; filename (sans .md) is the fallback name.
func (r *SkillRegistry) scanSkillDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	// Sort for deterministic order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, e := range entries {
		if e.IsDir() {
			// Allow skills/my-skill/SKILL.md pattern.
			subDir := filepath.Join(dir, e.Name())
			candidate := filepath.Join(subDir, "SKILL.md")
			if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
				if skill, err := parseSkillFile(candidate); err == nil && skill.Name != "" {
					if _, exists := r.skills[skill.Name]; !exists {
						r.skills[skill.Name] = skill
					}
				}
			}
			continue
		}

		if !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		skill, err := parseSkillFile(path)
		if err != nil || skill.Name == "" {
			continue
		}
		if _, exists := r.skills[skill.Name]; !exists {
			r.skills[skill.Name] = skill
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// SKILL.md parsing — YAML frontmatter + Markdown body
// ---------------------------------------------------------------------------

// parseSkillFile reads a skill file with optional YAML frontmatter.
//
// Supported formats:
//
//  1. agentskills.io YAML frontmatter:
//     ---
//     name: my-skill
//     description: ...
//     tools: [...]
//     ---
//     # Body instructions (Markdown)
//
//  2. Legacy (no frontmatter): parsed by the heading-based heuristic.
func parseSkillFile(path string) (*Skill, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(raw)

	// Try YAML frontmatter first.
	if strings.HasPrefix(content, "---") {
		return parseFrontmatterSkill(content, path)
	}

	// Fallback to legacy heading-based parser.
	return parseLegacySkill(content, path)
}

// parseFrontmatterSkill parses a SKILL.md with YAML frontmatter.
func parseFrontmatterSkill(content, path string) (*Skill, error) {
	// Split on --- boundaries.
	parts := bytes.SplitN([]byte(content), []byte("\n---\n"), 2)
	if len(parts) < 2 {
		// Maybe the closing --- is at end without trailing newline.
		parts = bytes.SplitN([]byte(content), []byte("\n---"), 2)
		if len(parts) < 2 {
			// Only opening ---, no closing: treat as legacy.
			return parseLegacySkill(content, path)
		}
	}

	fmBytes := parts[0]
	bodyBytes := parts[1]

	// Strip leading ---
	fmBytes = bytes.TrimPrefix(fmBytes, []byte("---"))
	fmBytes = bytes.TrimSpace(fmBytes)

	var fm skillFrontmatter
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	skill := &Skill{
		Name:        fm.Name,
		Description: fm.Description,
		FilePath:    path,
		Body:        strings.TrimSpace(string(bodyBytes)),
		Tools:       make([]SkillTool, 0, len(fm.Tools)),
	}

	for _, ft := range fm.Tools {
		st := SkillTool{
			Name:        ft.Name,
			Description: ft.Description,
			Script:      ft.Script,
			Parameters:  make([]SkillParam, 0, len(ft.Parameters)),
			Required:    []string{},
		}
		for _, fp := range ft.Parameters {
			st.Parameters = append(st.Parameters, SkillParam{
				Name:        fp.Name,
				Type:        fp.Type,
				Description: fp.Description,
			})
			if fp.Required {
				st.Required = append(st.Required, fp.Name)
			}
		}
		skill.Tools = append(skill.Tools, st)
	}

	if skill.Name == "" {
		// Fallback: directory name (skills/my-skill/SKILL.md → my-skill)
		skill.Name = filepath.Base(filepath.Dir(path))
		if skill.Name == "skills" || skill.Name == ".agents" {
			// Edge case: skills/my-skill.md → my-skill
			skill.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
	}

	return skill, nil
}

// parseLegacySkill handles the old heading-based SKILL.md format (no frontmatter).
func parseLegacySkill(content, path string) (*Skill, error) {
	lines := strings.Split(content, "\n")
	skill := &Skill{
		FilePath: path,
		Tools:    []SkillTool{},
	}

	var currentTool *SkillTool
	var bodyLines []string
	inTool := false
	inScript := false
	headerDone := false

	for _, line := range lines {
		if strings.HasPrefix(line, "# ") && skill.Name == "" {
			skill.Name = strings.TrimPrefix(line, "# ")
			continue
		}
		if strings.HasPrefix(line, "## ") && !inTool {
			section := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			if strings.EqualFold(section, "Description") {
				continue
			}
			if strings.EqualFold(section, "Tools") {
				continue
			}
			// Unknown section header → body
			headerDone = true
			bodyLines = append(bodyLines, line)
			continue
		}
		if !headerDone && skill.Description == "" && !strings.HasPrefix(line, "##") && !strings.HasPrefix(line, "-") && strings.TrimSpace(line) != "" {
			skill.Description = strings.TrimSpace(line)
			continue
		}
		if strings.HasPrefix(line, "- `") && strings.HasSuffix(line, "`") {
			if currentTool != nil {
				skill.Tools = append(skill.Tools, *currentTool)
			}
			toolName := strings.TrimPrefix(strings.TrimSuffix(line, "`"), "- `")
			currentTool = &SkillTool{
				Name:       toolName,
				Parameters: []SkillParam{},
				Required:   []string{},
			}
			inTool = true
			inScript = false
			headerDone = true
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
						currentTool.Parameters = append(currentTool.Parameters, SkillParam{
							Name:        paramName,
							Type:        paramType,
							Description: paramDesc,
						})
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
			continue
		}
		// Anything else after headers → body
		headerDone = true
		bodyLines = append(bodyLines, line)
	}

	if currentTool != nil {
		skill.Tools = append(skill.Tools, *currentTool)
	}

	if skill.Name == "" {
		skill.Name = filepath.Base(filepath.Dir(path))
	}

	skill.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))

	return skill, nil
}

// ---------------------------------------------------------------------------
// Registry operations
// ---------------------------------------------------------------------------

// ListSkills returns all discovered skills.
func (r *SkillRegistry) ListSkills() []*Skill {
	skills := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		skills = append(skills, s)
	}
	return skills
}

// GetSkill returns a skill by name.
func (r *SkillRegistry) GetSkill(name string) (*Skill, bool) {
	s, ok := r.skills[name]
	return s, ok
}

// LoadSkill loads a skill's tools into the ToolRegistry.
// When allowScripts is false (Analyze), script-backed tools are skipped;
// the skill body is still returned for progressive disclosure of instructions.
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
			continue
		}
		toolDef := skillToolToToolDef(st, r.sandbox)
		registry.Register(toolDef)
	}

	skill.Loaded = true
	if skippedScripts > 0 && !allowScripts {
		return fmt.Errorf("skill %q: %d script tool(s) blocked (analyze readonly); instructions loaded only", name, skippedScripts)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tool conversion
// ---------------------------------------------------------------------------

// skillToolToToolDef converts a SkillTool to a ToolDef.
func skillToolToToolDef(st SkillTool, sb *sandbox.Sandbox) *ToolDef {
	props := make(map[string]llm.Property)
	for _, p := range st.Parameters {
		props[p.Name] = llm.Property{
			Type:        p.Type,
			Description: p.Description,
		}
	}

	params := llm.Parameters{
		Type:       "object",
		Properties: props,
		Required:   st.Required,
	}

	return &ToolDef{
		Name:        st.Name,
		Description: st.Description,
		Parameters:  params,
		Fn: func(callParams map[string]interface{}) (string, error) {
			script := st.Script
			for name, val := range callParams {
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

// ---------------------------------------------------------------------------
// Agent tools: list_skills / load_skill
// ---------------------------------------------------------------------------

// NewListSkillsTool creates a tool that lists available skills.
// Progressive disclosure: only returns name + description (no tools, no body).
func NewListSkillsTool(skillReg *SkillRegistry) *ToolDef {
	return &ToolDef{
		Name:        "list_skills",
		Description: "List available skills discovered from SKILL.md files. Returns name and description only; use load_skill to access full instructions and tools.",
		Parameters: llm.Parameters{
			Type: "object",
		},
		Fn: func(params map[string]interface{}) (string, error) {
			if err := skillReg.ScanSkills(); err != nil {
				return fmt.Sprintf("Error scanning skills: %v", err), nil
			}

			skills := skillReg.ListSkills()
			if len(skills) == 0 {
				return "No skills found. Place SKILL.md files in skills/ or .agents/skills/ directories.", nil
			}

			var sb strings.Builder
			sb.WriteString("Available skills:\n")
			for _, s := range skills {
				status := "not loaded"
				if s.Loaded {
					status = "loaded"
				}
				sb.WriteString(fmt.Sprintf("- %s (%s): %s\n", s.Name, status, s.Description))
			}
			sb.WriteString("\nUse load_skill(name) to load a skill's instructions and tools.")
			return sb.String(), nil
		},
	}
}

// NewLoadSkillTool creates a tool that loads a specific skill.
// Progressive disclosure: returns the skill body (instructions) and registers tools.
// When allowScripts is false (Analyze), script tools are not registered.
func NewLoadSkillTool(skillReg *SkillRegistry, toolReg *ToolRegistry, allowScripts bool) *ToolDef {
	desc := "Load a skill: injects its instructions into the conversation and registers its tools for use."
	if !allowScripts {
		desc = "Load a skill for analysis. Script tools are blocked in analyze mode; only instructions and metadata are disclosed."
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
				if skill != nil && !allowScripts {
					sb.WriteString(fmt.Sprintf("Skill '%s' instructions loaded (scripts blocked).\n", name))
					if skill.Description != "" {
						sb.WriteString("Description: " + skill.Description + "\n")
					}
					if skill.Body != "" {
						sb.WriteString("\n--- Skill Instructions ---\n")
						sb.WriteString(skill.Body)
						sb.WriteString("\n--- End Instructions ---\n")
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
				// Inject body instructions
				if skill.Body != "" {
					sb.WriteString("\n--- Skill Instructions ---\n")
					sb.WriteString(skill.Body)
					sb.WriteString("\n--- End Instructions ---\n")
				}
				if allowScripts && len(skill.Tools) > 0 {
					sb.WriteString("\nAvailable tools:\n")
					for _, t := range skill.Tools {
						sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Name, t.Description))
					}
				}
			}
			return sb.String(), nil
		},
	}
}
