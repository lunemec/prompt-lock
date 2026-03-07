package main

import (
	"context"
	"net/http"
)

type ctxKey string

const (
	ctxActorType ctxKey = "actor_type"
	ctxActorID   ctxKey = "actor_id"
)

func actorFromRequest(r *http.Request) (string, string) {
	at, _ := r.Context().Value(ctxActorType).(string)
	id, _ := r.Context().Value(ctxActorID).(string)
	return at, id
}

func withActor(r *http.Request, actorType, actorID string) *http.Request {
	ctx := context.WithValue(r.Context(), ctxActorType, actorType)
	ctx = context.WithValue(ctx, ctxActorID, actorID)
	return r.WithContext(ctx)
}
