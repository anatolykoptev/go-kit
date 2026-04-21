package rerank_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/anatolykoptev/go-kit/rerank"
)

func ExampleClient_Rerank() {
	c := rerank.New(rerank.Config{
		URL:     os.Getenv("EMBED_SERVER_URL"),
		Model:   "gte-multi-rerank",
		Timeout: 4 * time.Second,
		MaxDocs: 20,
	}, nil)

	if !c.Available() {
		fmt.Println("rerank disabled")
		return
	}
	docs := []rerank.Doc{
		{ID: "u1", Text: "Go is a statically typed compiled language."},
		{ID: "u2", Text: "Python is a dynamically typed interpreted language."},
	}
	scored := c.Rerank(context.Background(), "what is Go?", docs)
	for _, s := range scored {
		fmt.Printf("%s: %.3f\n", s.ID, s.Score)
	}
}
