package document

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
	"github.com/stumble/wpgx"
)

func Test_QueryDocument(t *testing.T) {
	os.Setenv("POSTGRES_PASSWORD", "postgres")
	os.Setenv("POSTGRES_APPNAME", "novaforge")
	os.Setenv("POSTGRES_DBNAME", "llt_data_db")
	os.Setenv("POSTGRES_PASSWORD", "my-secret")
	var wpgxConfig wpgx.Config
	envconfig.MustProcess("postgres", &wpgxConfig)
	log.Info().Msgf("wpgx config: %+v", &wpgxConfig)

	pool, err := wpgx.NewPool(context.Background(), &wpgxConfig)
	if err != nil {
		t.Fatalf("failed to create wpgx pool: %v", err)
	}
	defer pool.Close()

	var authors []string = make([]string, 0)

	embedding := make([]float32, 1536)
	for i := range embedding {
		embedding[i] = float32(i)
	}

	db := New(pool.WConn(), nil)
	document, err := db.Create(context.Background(), CreateParams{
		Source:      "test",
		Catalog:     DocumentCatalogNews,
		Title:       "test",
		Content:     "test",
		Format:      DocumentFormatMarkdown,
		Authors:     authors,
		Url:         "test",
		Md5:         "test6",
		Status:      DocumentStatusActive,
		PublishedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to create document: %v", err)
	}
	log.Info().Msgf("document: %+v", document)

	count, err := db.UpdateEmbedding(context.Background(), UpdateEmbeddingParams{
		ID:        document.ID,
		Embedding: embedding,
	})
	if err != nil {
		t.Fatalf("failed to update embedding: %v", err)
	}
	log.Info().Msgf("embedding updated: %d", count)

	if count != 1 {
		t.Fatalf("failed to update embedding: %d", count)
	}

	document2, err := db.GetByIdWithEmbedding(context.Background(), document.ID)
	if err != nil {
		t.Fatalf("failed to get document: %v", err)
	}
	log.Info().Msgf("document: %+v", document2)
}
