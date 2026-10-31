package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"sandbox-pay/internal/httpx"
)

func (a *App) registerOrderDetails(r chi.Router) {
	r.Get("/transactions/{orderNo}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := a.c.Admin.GetTransaction(pathParam(r, "orderNo"))
		if err != nil {
			return err
		}
		httpx.OK(w, out)
		return nil
	}))
}
