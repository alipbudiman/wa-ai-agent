package pkg

import "github.com/google/uuid"

func GenerateUID() string {
	id := uuid.New()
	return id.String()
}
