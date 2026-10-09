package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Image 一条图片元数据；Name 既是磁盘文件名也是 URL 里的 key
type Image struct {
	Name     string    `json:"name"`
	Original string    `json:"original"`
	MIME     string    `json:"mime"`
	Size     int64     `json:"size"`
	Width    int       `json:"width"`
	Height   int       `json:"height"`
	Hash     string    `json:"hash"`
	At       time.Time `json:"at"`
}

// Store 用一个 JSON 文件当数据库。
// 几百张图的量级完全够用，而且备份就是拷这一个目录。
type Store struct {
	mu     sync.RWMutex
	dir    string
	path   string
	items  []*Image
	byName map[string]*Image
	byHash map[string]*Image
}

func NewStore(dir, indexPath string) (*Store, error) {
	s := &Store{
		dir:    dir,
		path:   indexPath,
		byName: map[string]*Image{},
		byHash: map[string]*Image{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var items []*Image
	if err := json.Unmarshal(b, &items); err != nil {
		return err
	}
	s.items = items
	for _, it := range items {
		if it.Name == "" {
			continue
		}
		s.byName[it.Name] = it
		if it.Hash != "" {
			s.byHash[it.Hash] = it
		}
	}
	return nil
}

// persistLocked 先写临时文件再 rename，避免断电写坏索引
func (s *Store) persistLocked() error {
	b, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) ByName(name string) *Image {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byName[name]
}

func (s *Store) ByHash(hash string) *Image {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byHash[hash]
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// Missing 统计索引里有、磁盘上没有的条目（多半是数据卷没挂对）
func (s *Store) Missing() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, it := range s.items {
		if _, err := os.Stat(filepath.Join(s.dir, it.Name)); err != nil {
			n++
		}
	}
	return n
}

func (s *Store) Add(img *Image) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, img)
	s.byName[img.Name] = img
	if img.Hash != "" {
		s.byHash[img.Hash] = img
	}
	return s.persistLocked()
}

func (s *Store) Delete(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	img := s.byName[name]
	if img == nil {
		return false, nil
	}
	delete(s.byName, name)
	if img.Hash != "" && s.byHash[img.Hash] == img {
		delete(s.byHash, img.Hash)
	}
	for i, it := range s.items {
		if it == img {
			s.items = append(s.items[:i], s.items[i+1:]...)
			break
		}
	}
	if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("删除文件 %s 失败: %v", name, err)
	}
	return true, s.persistLocked()
}

// List 按时间倒序返回，q 同时匹配原始文件名和存储文件名
func (s *Store) List(q string, offset, limit int) ([]*Image, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q = strings.ToLower(strings.TrimSpace(q))
	matched := make([]*Image, 0, len(s.items))
	for _, it := range s.items {
		if q == "" ||
			strings.Contains(strings.ToLower(it.Original), q) ||
			strings.Contains(strings.ToLower(it.Name), q) {
			matched = append(matched, it)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].At.After(matched[j].At) })

	total := len(matched)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return nil, total
	}
	end := total
	if limit > 0 && offset+limit < total {
		end = offset + limit
	}
	return matched[offset:end], total
}
