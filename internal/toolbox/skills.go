package toolbox

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxSkillBytes = 256 << 10
const maxInstructionBytes = 1 << 20

// Skills are plain instructions, never executable plugins.
type Skill struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
}
type SkillSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	LoadError string `json:"loadError,omitempty"`
}
type skillMetadata struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func validateSkillText(text string) error {
	if len(text) > maxSkillBytes {
		return errors.New("Skill 正文不能超过 256 KiB")
	}
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return errors.New("Skill 必须是 UTF-8 纯文本，不能包含二进制内容")
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("请填写 Skill 正文")
	}
	return nil
}
func validateSkillName(name string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 80 || strings.ContainsAny(name, "\r\n\x00") {
		return errors.New("Skill 名称应为 1–80 个字符，不能包含换行")
	}
	return nil
}

// Reject links at each app-managed boundary. This is local file hygiene, not a
// sandbox against another process racing filesystem changes.
func plainPath(path string, dir bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Skill 路径不能是符号链接")
	}
	if dir && !info.IsDir() || !dir && !info.Mode().IsRegular() {
		return errors.New("无效的 Skill 文件类型")
	}
	return nil
}
func (s *Service) skillDir(id string) (string, error) {
	if !validID.MatchString(id) {
		return "", errors.New("无效的 Skill ID")
	}
	library := filepath.Join(s.configDir, "skills")
	if err := plainPath(library, true); err != nil {
		return "", err
	}
	dir := filepath.Join(library, id)
	if err := plainPath(dir, true); err != nil {
		return "", err
	}
	return dir, nil
}
func readSkillText(path string) (string, error) {
	if err := plainPath(path, false); err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSkillBytes+1))
	if err != nil {
		return "", err
	}
	text := string(data)
	return text, validateSkillText(text)
}
func (s *Service) skill(id string) (Skill, error) {
	dir, err := s.skillDir(id)
	if err != nil {
		return Skill{}, err
	}
	metaPath := filepath.Join(dir, "skill.json")
	if err := plainPath(metaPath, false); err != nil {
		return Skill{}, err
	}
	var meta skillMetadata
	if err := readJSON(metaPath, &meta); err != nil {
		return Skill{}, err
	}
	if meta.ID != id {
		return Skill{}, errors.New("Skill ID 与目录不一致")
	}
	if err := validateSkillName(meta.Name); err != nil {
		return Skill{}, err
	}
	content, err := readSkillText(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return Skill{}, err
	}
	return Skill{ID: id, Name: meta.Name, Content: content}, nil
}
func (s *Service) GetSkill(id string) (Skill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.skill(id)
}

func (s *Service) listSkills() ([]SkillSummary, error) {
	result := []SkillSummary{}
	library := filepath.Join(s.configDir, "skills")
	if err := plainPath(library, true); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return result, nil
		}
		return result, err
	}
	entries, err := os.ReadDir(library)
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if !validID.MatchString(entry.Name()) {
			continue
		}
		item := SkillSummary{ID: entry.Name(), Name: entry.Name()}
		skill, err := s.skill(item.ID)
		if err != nil {
			item.LoadError = err.Error()
		} else {
			item.Name = skill.Name
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func writeSkillText(path, content string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".skill-*.tmp")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
func hasSkill(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}
func (s *Service) saveSkill(input Skill) (Skill, error) {
	input.Name = strings.TrimSpace(input.Name)
	if err := validateSkillName(input.Name); err != nil {
		return Skill{}, err
	}
	if err := validateSkillText(input.Content); err != nil {
		return Skill{}, err
	}
	if input.ID != "" {
		dir, err := s.skillDir(input.ID)
		if err != nil {
			return Skill{}, err
		}
		// Permit repairing a missing text/metadata file, never following a link.
		for _, name := range []string{"SKILL.md", "skill.json"} {
			if err := plainPath(filepath.Join(dir, name), false); err != nil && !errors.Is(err, os.ErrNotExist) {
				return Skill{}, err
			}
		}
		for id := range s.active {
			tool, err := s.tool(id)
			if err != nil {
				return Skill{}, err
			}
			if hasSkill(tool.SkillIDs, input.ID) {
				return Skill{}, fmt.Errorf("请先停止工具「%s」的请求，再编辑此 Skill", tool.Name)
			}
		}
		if err = writeSkillText(filepath.Join(dir, "SKILL.md"), input.Content); err != nil {
			return Skill{}, err
		}
		if err = writeJSON(filepath.Join(dir, "skill.json"), skillMetadata{ID: input.ID, Name: input.Name}); err != nil {
			return Skill{}, err
		}
		return input, nil
	}
	library := filepath.Join(s.configDir, "skills")
	if err := os.MkdirAll(library, 0700); err != nil {
		return Skill{}, err
	}
	if err := plainPath(library, true); err != nil {
		return Skill{}, err
	}
	id, err := newID()
	if err != nil {
		return Skill{}, err
	}
	input.ID = id
	dir, err := os.MkdirTemp(library, ".install-")
	if err != nil {
		return Skill{}, err
	}
	defer os.RemoveAll(dir)
	if err = writeSkillText(filepath.Join(dir, "SKILL.md"), input.Content); err != nil {
		return Skill{}, err
	}
	if err = writeJSON(filepath.Join(dir, "skill.json"), skillMetadata{ID: id, Name: input.Name}); err != nil {
		return Skill{}, err
	}
	if err = os.Rename(dir, filepath.Join(library, id)); err != nil {
		return Skill{}, err
	}
	return input, nil
}
func (s *Service) SaveSkill(input Skill) (Skill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveSkill(input)
}

func importedSkillName(text, path string) string {
	// Preserve the complete imported document; this only extracts a friendly name.
	lines := strings.Split(strings.ReplaceAll(strings.TrimPrefix(text, "\ufeff"), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[0] == "---" {
		for _, line := range lines[1:] {
			if line == "---" {
				break
			}
			if value, ok := strings.CutPrefix(line, "name:"); ok {
				value = strings.Trim(strings.TrimSpace(value), "\"'")
				if validateSkillName(value) == nil {
					return value
				}
			}
		}
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if strings.EqualFold(name, "SKILL") {
		name = "导入的 Skill"
	}
	return name
}
func (s *Service) PreviewSkillImport(path string) (Skill, error) {
	text, err := readSkillText(path)
	if err != nil {
		return Skill{}, err
	}
	return Skill{Name: importedSkillName(text, path), Content: text}, nil
}
func (s *Service) ImportSkill(path string) (Skill, error) {
	input, err := s.PreviewSkillImport(path)
	if err != nil {
		return Skill{}, err
	}
	return s.SaveSkill(input)
}
func (s *Service) DeleteSkill(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.skillDir(id)
	if err != nil {
		return err
	}
	for _, ref := range s.config.Tools {
		tool, err := loadTool(ref)
		if err != nil {
			return fmt.Errorf("先修复或移除不可读取的工作区，才能确认 Skill 引用：%w", err)
		}
		if hasSkill(tool.SkillIDs, id) {
			return fmt.Errorf("工具「%s」已获授权，请先在工具配置中取消授权，再删除此 Skill", tool.Name)
		}
	}
	return os.RemoveAll(dir)
}
func (s *Service) validateSkillIDs(ids []string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return errors.New("Skill 授权不能重复")
		}
		seen[id] = true
		if _, err := s.skill(id); err != nil {
			return fmt.Errorf("无法授权 Skill %s：%w", id, err)
		}
	}
	return nil
}
func (s *Service) instructions(t Tool) (string, error) {
	if len(t.SkillIDs) == 0 {
		return t.System, nil
	}
	var out strings.Builder
	out.WriteString(t.System)
	seen := map[string]bool{}
	for _, id := range t.SkillIDs {
		if seen[id] {
			return "", errors.New("Skill 授权重复，请检查工具设置")
		}
		seen[id] = true
		skill, err := s.skill(id)
		if err != nil {
			return "", fmt.Errorf("已授权的 Skill %s 无法读取，请修复或取消授权：%w", id, err)
		}
		fmt.Fprintf(&out, "\n\n---\n文本 Skill：%s（%s）\n仅作为文本任务说明，不执行脚本或外部操作。\n%s\n---", skill.Name, skill.ID, skill.Content)
		if out.Len() > maxInstructionBytes {
			return "", errors.New("System 与已授权 Skill 合计超过 1 MiB，请减少授权或正文长度")
		}
	}
	return out.String(), nil
}
