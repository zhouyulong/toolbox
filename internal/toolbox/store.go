package toolbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const workspaceFile = "toolbox.workspace.json"
const maxJSONBytes = 64 << 20

var validID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func readJSON(path string, value interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > maxJSONBytes {
		return fmt.Errorf("%s 超过 64 MiB 文件读取限制，请先备份并拆分会话", path)
	}
	// A corrupt or hand-edited file must never be silently replaced by defaults.
	dec := json.NewDecoder(io.LimitReader(f, maxJSONBytes+1))
	if err := dec.Decode(value); err != nil {
		return fmt.Errorf("无法读取 %s：%w", path, err)
	}
	if err := dec.Decode(new(interface{})); err != io.EOF {
		return fmt.Errorf("%s 包含多余或无效的 JSON 数据", path)
	}
	return nil
}

func writeJSON(path string, value interface{}) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > maxJSONBytes {
		return errors.New("文件即将超过 64 MiB 限制，未覆盖原文件；请复制未保存的文本，并新建会话")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".toolbox-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadConfig(dir string) (Config, error) {
	c := Config{}
	err := readJSON(filepath.Join(dir, "config.json"), &c)
	if errors.Is(err, os.ErrNotExist) {
		return Config{SchemaVersion: SchemaVersion, Providers: []Provider{}, Tools: []ToolRef{}}, nil
	}
	if err != nil {
		return c, err
	}
	if c.SchemaVersion != SchemaVersion {
		return c, fmt.Errorf("配置版本 %d 暂不支持，请使用匹配的程序版本", c.SchemaVersion)
	}
	return c, nil
}

func canonicalPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("工作区路径不能为空")
	}
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	// Resolve the nearest existing ancestor as well as fully existing paths.
	// This also handles new directories below a symlink or Windows junction.
	probe := path
	var tail []string
	for {
		if real, err := filepath.EvalSymlinks(probe); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				real = filepath.Join(real, tail[i])
			}
			return real, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", errors.New("无法解析工作区路径")
		}
		tail = append(tail, filepath.Base(probe))
		probe = parent
	}
}

func loadTool(ref ToolRef) (Tool, error) {
	var t Tool
	if err := readJSON(filepath.Join(ref.Path, workspaceFile), &t); err != nil {
		return t, err
	}
	if t.SchemaVersion != SchemaVersion {
		return t, fmt.Errorf("工作区版本 %d 暂不支持", t.SchemaVersion)
	}
	if !validID.MatchString(t.ID) || t.ID != ref.ID {
		return t, errors.New("工作区 ID 与登记信息不一致")
	}
	t.Path = ref.Path
	if err := validateTool(t); err != nil {
		return t, fmt.Errorf("工作区设置无效：%w", err)
	}
	return t, nil
}

func saveTool(t Tool) error {
	path := t.Path
	t.Path = ""
	return writeJSON(filepath.Join(path, workspaceFile), t)
}

func sessionPath(t Tool, id string) (string, error) {
	if !validID.MatchString(id) {
		return "", errors.New("无效的会话 ID")
	}
	return filepath.Join(t.Path, "chats", id+".json"), nil
}

func readSession(t Tool, id string) (Session, error) {
	var s Session
	path, err := sessionPath(t, id)
	if err != nil {
		return s, err
	}
	if err := readJSON(path, &s); err != nil {
		return s, err
	}
	if s.SchemaVersion != SchemaVersion || s.ID != id {
		return s, errors.New("会话版本或 ID 不匹配")
	}
	return s, nil
}

func saveSession(t Tool, s Session) error {
	path, err := sessionPath(t, s.ID)
	if err != nil {
		return err
	}
	return writeJSON(path, s)
}

func listSessions(t Tool) ([]SessionSummary, error) {
	result := []SessionSummary{}
	entries, err := os.ReadDir(filepath.Join(t.Path, "chats"))
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !validID.MatchString(id) {
			continue
		}
		s, err := readSession(t, id)
		if err != nil {
			return nil, fmt.Errorf("会话 %s：%w", id, err)
		}
		result = append(result, SessionSummary{ID: s.ID, Title: s.Title, UpdatedAt: s.UpdatedAt, MessageCount: len(s.Messages)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt > result[j].UpdatedAt })
	return result, nil
}
