package api

import (
	"net/http"
	"time"

	"github.com/mattparavel/a-git/internal/review"
)

// modTime is used with http.ServeContent; objects are immutable so no
// Last-Modified header is needed.
var modTime time.Time

// ReviewCommentRequest adds a comment.
type ReviewCommentRequest struct {
	Body string `json:"body"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

// ReviewUploadRequest adds a revision.
type ReviewUploadRequest struct {
	Head string `json:"head"`
}

// ReviewsResponse lists reviews.
type ReviewsResponse struct {
	Reviews []*review.Review `json:"reviews"`
}

// ReviewResponse returns a review with its evaluated status.
type ReviewResponse struct {
	Review *review.Review `json:"review"`
	Status *review.Status `json:"status,omitempty"`
}

func (s *Server) reviewRoutes() {
	s.handle("POST /v1/repos/{repo}/reviews", true, s.createReview)
	s.handle("GET /v1/repos/{repo}/reviews", true, s.listReviews)
	s.handle("GET /v1/repos/{repo}/reviews/{id}", true, s.getReview)
	s.handle("POST /v1/repos/{repo}/reviews/{id}/revisions", true, s.uploadReview)
	s.handle("POST /v1/repos/{repo}/reviews/{id}/comments", true, s.commentReview)
	s.handle("POST /v1/repos/{repo}/reviews/{id}/approve", true, s.approveReview)
	s.handle("POST /v1/repos/{repo}/reviews/{id}/abandon", true, s.abandonReview)
	s.handle("POST /v1/repos/{repo}/reviews/{id}/land", true, s.landReview)
	s.handle("GET /v1/repos/{repo}/landed", true, s.findReview)
}

func (s *Server) reviewResponse(w http.ResponseWriter, r *http.Request, c *reqCtx, rv *review.Review) error {
	st, err := s.reviews.Evaluate(r.Context(), c.repo.Name, rv.ID)
	if err != nil {
		return err
	}
	mutable(w)
	writeJSON(w, http.StatusOK, ReviewResponse{Review: rv, Status: st})
	return nil
}

func reviewID(r *http.Request) (int, error) { return review.ParseID(r.PathValue("id")) }

func (s *Server) createReview(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	var req review.CreateRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	rv, err := s.reviews.Create(r.Context(), c.repo.Name, c.pr, req)
	if err != nil {
		return err
	}
	return s.reviewResponse(w, r, c, rv)
}

func (s *Server) listReviews(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	list, err := s.reviews.List(r.Context(), c.repo.Name, r.URL.Query().Get("state"), queryInt(r, "limit", 100))
	if err != nil {
		return err
	}
	if list == nil {
		list = []*review.Review{}
	}
	mutable(w)
	writeJSON(w, http.StatusOK, ReviewsResponse{Reviews: list})
	return nil
}

func (s *Server) getReview(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	id, err := reviewID(r)
	if err != nil {
		return err
	}
	rv, err := s.reviews.Get(r.Context(), c.repo.Name, id)
	if err != nil {
		return err
	}
	return s.reviewResponse(w, r, c, rv)
}

func (s *Server) uploadReview(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	id, err := reviewID(r)
	if err != nil {
		return err
	}
	var req ReviewUploadRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	rv, err := s.reviews.Upload(r.Context(), c.repo.Name, id, c.pr, req.Head)
	if err != nil {
		return err
	}
	return s.reviewResponse(w, r, c, rv)
}

func (s *Server) commentReview(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	id, err := reviewID(r)
	if err != nil {
		return err
	}
	var req ReviewCommentRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	rv, err := s.reviews.Comment(r.Context(), c.repo.Name, id, c.pr, req.Body, req.Path, req.Line)
	if err != nil {
		return err
	}
	return s.reviewResponse(w, r, c, rv)
}

func (s *Server) approveReview(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	id, err := reviewID(r)
	if err != nil {
		return err
	}
	rv, err := s.reviews.Approve(r.Context(), c.repo.Name, id, c.pr)
	if err != nil {
		return err
	}
	return s.reviewResponse(w, r, c, rv)
}

func (s *Server) abandonReview(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	id, err := reviewID(r)
	if err != nil {
		return err
	}
	rv, err := s.reviews.Abandon(r.Context(), c.repo.Name, id, c.pr)
	if err != nil {
		return err
	}
	return s.reviewResponse(w, r, c, rv)
}

func (s *Server) landReview(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	res, err := s.reviews.Land(r.Context(), c.repo.Name, r.PathValue("id"), c.pr)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}

func (s *Server) findReview(w http.ResponseWriter, r *http.Request, c *reqCtx) error {
	cs, err := s.resolve(r.Context(), c, r.URL.Query().Get("changeset"))
	if err != nil {
		return err
	}
	rv, err := s.reviews.FindByChangeset(r.Context(), c.repo.Name, cs)
	if err != nil {
		return err
	}
	return s.reviewResponse(w, r, c, rv)
}
