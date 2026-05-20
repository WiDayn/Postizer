package site

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxCommentNicknameLength = 80
	maxCommentEmailLength    = 254
	maxCommentBodyLength     = 4000
	maxCommentReplyLength    = 4000
)

type CommentSettings struct {
	Enabled bool `json:"enabled"`
}

type CommentInput struct {
	PostSlug string `json:"post_slug"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Body     string `json:"body"`
}

type Comment struct {
	ID       string       `json:"id"`
	PostSlug string       `json:"post_slug"`
	Nickname string       `json:"nickname"`
	Email    string       `json:"email"`
	Body     string       `json:"body"`
	Created  time.Time    `json:"created"`
	Reply    CommentReply `json:"reply,omitempty"`
}

type CommentReply struct {
	Body    string    `json:"body,omitempty"`
	Created time.Time `json:"created,omitempty"`
}

type commentsFile struct {
	Comments []Comment `json:"comments"`
}

func AddComment(root string, input CommentInput, now time.Time) (Comment, error) {
	input, err := normalizeCommentInput(input)
	if err != nil {
		return Comment{}, err
	}
	comments, err := LoadComments(root)
	if err != nil {
		return Comment{}, err
	}
	id, err := uniqueCommentID(comments)
	if err != nil {
		return Comment{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	comment := Comment{
		ID:       id,
		PostSlug: input.PostSlug,
		Nickname: input.Nickname,
		Email:    input.Email,
		Body:     input.Body,
		Created:  now.Truncate(time.Second),
	}
	comments = append(comments, comment)
	if err := SaveComments(root, comments); err != nil {
		return Comment{}, err
	}
	return comment, nil
}

func ReplyToComment(root, id, body string, now time.Time) (Comment, error) {
	id = strings.TrimSpace(id)
	body = strings.TrimSpace(body)
	if id == "" {
		return Comment{}, fmt.Errorf("comment id is required")
	}
	if body == "" {
		return Comment{}, fmt.Errorf("reply is required")
	}
	if utf8Len(body) > maxCommentReplyLength {
		return Comment{}, fmt.Errorf("reply must be %d characters or fewer", maxCommentReplyLength)
	}
	comments, err := LoadComments(root)
	if err != nil {
		return Comment{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	for i := range comments {
		if comments[i].ID != id {
			continue
		}
		comments[i].Reply = CommentReply{
			Body:    body,
			Created: now.Truncate(time.Second),
		}
		if err := SaveComments(root, comments); err != nil {
			return Comment{}, err
		}
		return comments[i], nil
	}
	return Comment{}, fmt.Errorf("comment not found")
}

func MoveComments(root, oldSlug, newSlug string) error {
	oldSlug = strings.TrimSpace(oldSlug)
	newSlug = strings.TrimSpace(newSlug)
	if oldSlug == "" || oldSlug == newSlug {
		return nil
	}
	if !ValidSlug(oldSlug) || !ValidSlug(newSlug) {
		return nil
	}
	comments, err := LoadComments(root)
	if err != nil {
		return err
	}
	changed := false
	for i := range comments {
		if comments[i].PostSlug == oldSlug {
			comments[i].PostSlug = newSlug
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return SaveComments(root, comments)
}

func LoadComments(root string) ([]Comment, error) {
	path := commentsPath(root)
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var file commentsFile
	if err := json.Unmarshal(body, &file); err != nil {
		var legacy []Comment
		if legacyErr := json.Unmarshal(body, &legacy); legacyErr != nil {
			return nil, err
		}
		file.Comments = legacy
	}
	comments := make([]Comment, 0, len(file.Comments))
	for _, comment := range file.Comments {
		normalized, ok := normalizeStoredComment(comment)
		if ok {
			comments = append(comments, normalized)
		}
	}
	return comments, nil
}

func SaveComments(root string, comments []Comment) error {
	normalized := make([]Comment, 0, len(comments))
	for _, comment := range comments {
		if next, ok := normalizeStoredComment(comment); ok {
			normalized = append(normalized, next)
		}
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(commentsFile{Comments: normalized}, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(commentsPath(root), body, 0644)
}

func CommentsForPost(root, postSlug string) ([]Comment, error) {
	postSlug = strings.TrimSpace(postSlug)
	if postSlug == "" {
		return nil, nil
	}
	comments, err := LoadComments(root)
	if err != nil {
		return nil, err
	}
	filtered := make([]Comment, 0, len(comments))
	for _, comment := range comments {
		if comment.PostSlug == postSlug {
			filtered = append(filtered, comment)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].Created.Before(filtered[j].Created)
	})
	return filtered, nil
}

func SortCommentsNewestFirst(comments []Comment) {
	sort.SliceStable(comments, func(i, j int) bool {
		return comments[i].Created.After(comments[j].Created)
	})
}

func defaultCommentSettings() CommentSettings {
	return CommentSettings{Enabled: false}
}

func normalizeCommentSettings(settings CommentSettings) CommentSettings {
	return CommentSettings{Enabled: settings.Enabled}
}

func normalizeCommentInput(input CommentInput) (CommentInput, error) {
	input.PostSlug = strings.TrimSpace(input.PostSlug)
	input.Nickname = strings.TrimSpace(input.Nickname)
	input.Email = strings.TrimSpace(input.Email)
	input.Body = strings.TrimSpace(input.Body)
	if !ValidSlug(input.PostSlug) {
		return CommentInput{}, fmt.Errorf("invalid post slug")
	}
	if input.Nickname == "" {
		return CommentInput{}, fmt.Errorf("nickname is required")
	}
	if utf8Len(input.Nickname) > maxCommentNicknameLength {
		return CommentInput{}, fmt.Errorf("nickname must be %d characters or fewer", maxCommentNicknameLength)
	}
	if input.Email == "" {
		return CommentInput{}, fmt.Errorf("email is required")
	}
	if utf8Len(input.Email) > maxCommentEmailLength {
		return CommentInput{}, fmt.Errorf("email must be %d characters or fewer", maxCommentEmailLength)
	}
	address, err := mail.ParseAddress(input.Email)
	if err != nil {
		return CommentInput{}, fmt.Errorf("invalid email")
	}
	input.Email = address.Address
	if input.Body == "" {
		return CommentInput{}, fmt.Errorf("comment is required")
	}
	if utf8Len(input.Body) > maxCommentBodyLength {
		return CommentInput{}, fmt.Errorf("comment must be %d characters or fewer", maxCommentBodyLength)
	}
	return input, nil
}

func normalizeStoredComment(comment Comment) (Comment, bool) {
	comment.ID = strings.TrimSpace(comment.ID)
	comment.PostSlug = strings.TrimSpace(comment.PostSlug)
	comment.Nickname = strings.TrimSpace(comment.Nickname)
	comment.Email = strings.TrimSpace(comment.Email)
	comment.Body = strings.TrimSpace(comment.Body)
	comment.Reply.Body = strings.TrimSpace(comment.Reply.Body)
	if comment.ID == "" || !ValidSlug(comment.PostSlug) || comment.Nickname == "" || comment.Email == "" || comment.Body == "" {
		return Comment{}, false
	}
	address, err := mail.ParseAddress(comment.Email)
	if err != nil {
		return Comment{}, false
	}
	comment.Email = address.Address
	if utf8Len(comment.Nickname) > maxCommentNicknameLength {
		comment.Nickname = trimRunes(comment.Nickname, maxCommentNicknameLength)
	}
	if utf8Len(comment.Email) > maxCommentEmailLength {
		comment.Email = trimRunes(comment.Email, maxCommentEmailLength)
	}
	if utf8Len(comment.Body) > maxCommentBodyLength {
		comment.Body = trimRunes(comment.Body, maxCommentBodyLength)
	}
	if utf8Len(comment.Reply.Body) > maxCommentReplyLength {
		comment.Reply.Body = trimRunes(comment.Reply.Body, maxCommentReplyLength)
	}
	if comment.Reply.Body == "" {
		comment.Reply = CommentReply{}
	}
	return comment, true
}

func uniqueCommentID(comments []Comment) (string, error) {
	used := map[string]bool{}
	for _, comment := range comments {
		used[comment.ID] = true
	}
	for attempts := 0; attempts < 10; attempts++ {
		id, err := randomHexID(16)
		if err != nil {
			return "", err
		}
		if !used[id] {
			return id, nil
		}
	}
	return "", fmt.Errorf("could not allocate comment id")
}

func randomHexID(size int) (string, error) {
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return hex.EncodeToString(body), nil
}

func commentsPath(root string) string {
	return filepath.Join(root, "comments.json")
}

func utf8Len(value string) int {
	return len([]rune(value))
}

func trimRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
