package httpapi

import (
	"context"
	"errors"
	"github.com/quests-inc/quests/go/internal/store"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

func (s *Server) registerContainerTrash(mux *http.ServeMux) {
	for _, kind := range []string{"template", "questline"} {
		seg := "/api/trash/" + kind + "s"
		mux.HandleFunc("GET "+seg, func(w http.ResponseWriter, r *http.Request) {
			out, err := s.Store.ListTrashedContainers(r.Context(), kind)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, out)
		})
		mux.HandleFunc("POST "+seg+"/{id}/restore", func(w http.ResponseWriter, r *http.Request) {
			if !s.identityHTTP().RequireOwner(w, r, "восстанавливать из корзины") {
				return
			}
			id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
			if err != nil || id < 1 {
				writeErr(w, 400, "неверный номер")
				return
			}
			s.containerResult(w, s.Store.RestoreContainer(r.Context(), kind, id))
		})
		mux.HandleFunc("DELETE "+seg+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			if !s.identityHTTP().RequireOwner(w, r, "очищать корзину") {
				return
			}
			id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
			if err != nil || id < 1 {
				writeErr(w, 400, "неверный номер")
				return
			}
			s.containerResult(w, s.purgeContainer(r.Context(), kind, id))
		})
		mux.HandleFunc("DELETE "+seg, func(w http.ResponseWriter, r *http.Request) {
			if !s.identityHTTP().RequireOwner(w, r, "очищать корзину") {
				return
			}
			items, err := s.Store.ListTrashedContainers(r.Context(), kind)
			if err != nil {
				s.containerResult(w, err)
				return
			}
			for _, i := range items {
				if err := s.purgeContainer(r.Context(), kind, i.ID); err != nil {
					s.containerResult(w, err)
					return
				}
			}
			w.WriteHeader(204)
		})
	}
}
func (s *Server) containerResult(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "сущности нет в корзине")
	} else if err != nil {
		writeErr(w, 500, err.Error())
	} else {
		w.WriteHeader(204)
	}
}
func (s *Server) purgeContainer(ctx context.Context, kind string, id int64) error {
	icon, _ := s.Store.TrashedContainerIcon(ctx, kind, id)
	if err := s.Store.PurgeContainer(ctx, kind, id); err != nil {
		return err
	}
	if kind == "questline" {
		s.knowledgeHTTP().PurgeOwnerAttachments(ctx, kind, id)
		if icon != "" {
			_ = os.Remove(filepath.Join(s.DataDir, "questline-icons", icon))
		}
	}
	return nil
}
