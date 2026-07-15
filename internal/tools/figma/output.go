package figma

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const maxStreamedResponseBytes = 1 << 30

func (s *Service) emit(body []byte) error {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		body = []byte("{}")
	} else if !json.Valid(body) {
		return fmt.Errorf("figma: response is not valid JSON")
	}
	_, err := s.stdout().Write(append(body, '\n'))
	return err
}

func (s *Service) emitJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode Figma output: %w", err)
	}
	return s.emit(body)
}

func (s *Service) emitStream(reader io.Reader) error {
	temporary, err := os.CreateTemp("", ".anycli-figma-response-*")
	if err != nil {
		return fmt.Errorf("figma: create response spool: %w", err)
	}
	path := temporary.Name()
	defer os.Remove(path)
	defer temporary.Close()

	written, err := io.Copy(temporary, io.LimitReader(reader, maxStreamedResponseBytes+1))
	if err != nil {
		return fmt.Errorf("figma: spool response: %w", err)
	}
	if written > maxStreamedResponseBytes {
		return fmt.Errorf("figma: response exceeds %d bytes", maxStreamedResponseBytes)
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("figma: rewind response spool: %w", err)
	}
	hasJSON, err := validateJSONStream(temporary)
	if err != nil {
		return fmt.Errorf("figma: response is not valid JSON: %w", err)
	}
	if !hasJSON {
		_, err = io.WriteString(s.stdout(), "{}\n")
		return err
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("figma: rewind response spool: %w", err)
	}
	if _, err := io.Copy(s.stdout(), temporary); err != nil {
		return err
	}
	_, err = io.WriteString(s.stdout(), "\n")
	return err
}

func validateJSONStream(reader io.Reader) (bool, error) {
	decoder := json.NewDecoder(reader)
	started := false
	complete := false
	depth := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			if started && !complete {
				return false, io.ErrUnexpectedEOF
			}
			return started, nil
		}
		if err != nil {
			return false, err
		}
		if complete {
			return false, fmt.Errorf("multiple top-level JSON values")
		}
		started = true
		delimiter, isDelimiter := token.(json.Delim)
		if !isDelimiter {
			if depth == 0 {
				complete = true
			}
			continue
		}
		switch delimiter {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				complete = true
			}
		}
	}
}
