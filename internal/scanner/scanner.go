package scanner

import (
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/timae/rel.ai/internal/db"
	"github.com/timae/rel.ai/internal/model"
)

type SessionFile struct {
	TranscriptPath string
	SourceType     model.Source
	SourceID       string
	Mtime          int64
	Size           int64
}

type Scanner interface {
	Discover() ([]SessionFile, error)
	Parse(sf SessionFile) (*model.Session, []model.Message, error)
}

func ShortID(sourceType model.Source, sourceID string) string {
	h := sha256.Sum256([]byte(string(sourceType) + ":" + sourceID))
	return fmt.Sprintf("%x", h[:4])
}

type Orchestrator struct {
	DB       *db.DB
	Scanners []Scanner
}

func NewOrchestrator(store *db.DB, scanners ...Scanner) *Orchestrator {
	return &Orchestrator{DB: store, Scanners: scanners}
}

func (o *Orchestrator) Scan(full bool) (newCount, updatedCount int, err error) {
	for _, scanner := range o.Scanners {
		files, err := scanner.Discover()
		if err != nil {
			return newCount, updatedCount, fmt.Errorf("discovering sessions: %w", err)
		}

		for _, sf := range files {
			existing, _ := o.DB.GetBySourceID(sf.SourceType, sf.SourceID)

			if existing != nil && !full {
				if existing.TranscriptMtime.Unix() == sf.Mtime && existing.TranscriptSize == sf.Size {
					continue
				}
				o.DB.DeleteSession(existing.ID)
				updatedCount++
			} else if existing != nil && full {
				o.DB.DeleteSession(existing.ID)
				updatedCount++
			} else {
				newCount++
			}

			session, messages, err := scanner.Parse(sf)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", sf.SourceID, err)
				if existing == nil {
					newCount-- // undo the count
				}
				continue
			}

			if err := o.DB.InsertSession(session, messages); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to insert %s: %v\n", sf.SourceID, err)
			}
		}
	}

	return newCount, updatedCount, nil
}
