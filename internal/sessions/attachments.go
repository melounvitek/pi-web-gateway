package sessions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const attachmentMatchWindow = 5 * time.Minute

type Attachment struct {
	MessageHash string   `json:"message_hash"`
	Timestamp   string   `json:"timestamp"`
	Count       int      `json:"count"`
	Paths       []string `json:"paths"`
	MIMETypes   []string `json:"mime_types"`
}

type AttachmentMatch struct {
	Count  int
	Images []Image
}

type AttachmentStore struct{ Root string }

func (store AttachmentStore) Migrate(fromSessionPath, toSessionPath string) (func() error, error) {
	if fromSessionPath == toSessionPath {
		return nil, nil
	}
	fromPath := filepath.Join(store.Root, SessionHash(fromSessionPath)+".jsonl")
	from, err := os.ReadFile(fromPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(store.Root, 0700); err != nil {
		return nil, err
	}
	toPath := filepath.Join(store.Root, SessionHash(toSessionPath)+".jsonl")
	existing, err := os.ReadFile(toPath)
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(from) > 0 && bytes.HasSuffix(existing, from) {
		return nil, nil
	}
	if err := replaceFile(toPath, append(existing, from...)); err != nil {
		return nil, err
	}
	rollback := func() error {
		if !existed {
			err := os.Remove(toPath)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		return replaceFile(toPath, existing)
	}
	return rollback, nil
}

func replaceFile(path string, contents []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".attachment-migration-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err = temporary.Chmod(0600); err == nil {
		_, err = temporary.Write(contents)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func (store AttachmentStore) Match(sessionPath string, messages []*Message) map[*Message]AttachmentMatch {
	attachments := store.read(sessionPath)
	matches := make(map[*Message]AttachmentMatch)
	used := make(map[int]bool)
	for _, message := range messages {
		if message.Role != "user" {
			continue
		}
		hash := MessageHash(message.Text)
		best := -1
		bestDistance := attachmentMatchWindow + time.Nanosecond
		for index, attachment := range attachments {
			if used[index] || attachment.MessageHash != hash {
				continue
			}
			when, _ := time.Parse(time.RFC3339Nano, attachment.Timestamp)
			if message.Timestamp.IsZero() || when.IsZero() {
				if best < 0 {
					best = index
				}
				continue
			}
			distance := message.Timestamp.Sub(when)
			if distance < 0 {
				distance = -distance
			}
			if distance <= attachmentMatchWindow && distance < bestDistance {
				best, bestDistance = index, distance
			}
		}
		if best < 0 {
			continue
		}
		used[best] = true
		attachment := attachments[best]
		match := AttachmentMatch{Count: attachment.Count}
		for index, path := range attachment.Paths {
			mimeType := ""
			if index < len(attachment.MIMETypes) {
				mimeType = attachment.MIMETypes[index]
			}
			relative, err := filepath.Rel(store.Root, path)
			if err != nil || relative == ".." || len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator) {
				continue
			}
			match.Images = append(match.Images, Image{Src: "/attachments/" + filepath.ToSlash(relative), MIMEType: mimeType})
		}
		matches[message] = match
	}
	return matches
}

func (store AttachmentStore) read(sessionPath string) []Attachment {
	file, err := os.Open(filepath.Join(store.Root, SessionHash(sessionPath)+".jsonl"))
	if err != nil {
		return nil
	}
	defer file.Close()
	var result []Attachment
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var attachment Attachment
		if json.Unmarshal(scanner.Bytes(), &attachment) == nil {
			result = append(result, attachment)
		}
	}
	return result
}
