package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mattparavel/a-git/internal/api"
	"github.com/mattparavel/a-git/internal/review"
)

func registerReview() {
	register("review", "create|update|show|list|approve|comment|land|abandon|find ...", "code review with semantic approval rules", cmdReview)
}

func cmdReview(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return flag2("usage: agit review create|update|show|list|approve|comment|land|abandon|find")
	}
	op, args := args[0], args[1:]
	fs := newFlags("review "+op, "")
	title := fs.String("m", "", "title (create) or comment body (comment)")
	desc := fs.String("d", "", "description (create)")
	targetRef := fs.String("target", "", "target ref (create; default: the working copy's target, or main)")
	head := fs.String("head", "", "head revision (create/update; default: the working copy base)")
	state := fs.String("state", "open", "filter for list (open, landed, abandoned, or \"\" for all)")
	path := fs.String("path", "", "file a comment refers to")
	line := fs.Int("line", 0, "line a comment refers to")
	asJSON := fs.Bool("json", false, "machine-readable output")
	rf := addRepoFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	t, err := rf.open()
	if err != nil {
		return err
	}
	needID := func() (string, error) {
		if len(pos) != 1 {
			return "", flag2("review " + op + " needs a review ID")
		}
		return pos[0], nil
	}
	headRev := *head
	if headRev == "" {
		headRev = t.defaultRev()
	} else {
		headRev = t.rev(headRev)
	}
	var resp *api.ReviewResponse
	switch op {
	case "create":
		tgt := *targetRef
		if tgt == "" {
			tgt = "main"
			if t.wc != nil {
				tgt = t.wc.Config.Target
			}
		}
		resp, err = t.repo.CreateReview(ctx, review.CreateRequest{Title: *title, Description: *desc, Head: headRev, Target: tgt})
	case "update":
		id, err := needID()
		if err != nil {
			return err
		}
		resp, err = t.repo.UploadReview(ctx, id, headRev)
		if err != nil {
			return err
		}
	case "show":
		id, err := needID()
		if err != nil {
			return err
		}
		if resp, err = t.repo.Review(ctx, id); err != nil {
			return err
		}
	case "find":
		if len(pos) != 1 {
			return flag2("review find needs a revision")
		}
		resp, err = t.repo.FindReview(ctx, t.rev(pos[0]))
	case "approve":
		id, err := needID()
		if err != nil {
			return err
		}
		if resp, err = t.repo.ApproveReview(ctx, id); err != nil {
			return err
		}
	case "comment":
		id, err := needID()
		if err != nil {
			return err
		}
		if resp, err = t.repo.CommentReview(ctx, id, api.ReviewCommentRequest{Body: *title, Path: *path, Line: *line}); err != nil {
			return err
		}
	case "abandon":
		id, err := needID()
		if err != nil {
			return err
		}
		if resp, err = t.repo.AbandonReview(ctx, id); err != nil {
			return err
		}
	case "land":
		id, err := needID()
		if err != nil {
			return err
		}
		res, err := t.repo.LandReview(ctx, id)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(res)
		}
		fmt.Printf("landed review %d on %s: %s -> %s\n", res.Review.ID, res.Land.Ref, short(res.Land.Old), short(res.Land.New))
		if t.wc != nil {
			if err := t.wc.Lock(); err == nil {
				defer t.wc.Unlock()
				if upd, err := t.wc.Update(ctx, res.Land.New, false); err == nil {
					printUpdate(upd)
				}
			}
		}
		return nil
	case "list":
		list, err := t.repo.Reviews(ctx, *state)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(list)
		}
		for _, r := range list {
			fmt.Printf("R%-5d %-9s %-10s %s (%d revision(s))\n", r.ID, r.State, r.Author, r.Title, len(r.Revisions))
		}
		return nil
	default:
		return flag2("review: unknown operation " + op)
	}
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(resp)
	}
	printReview(resp)
	return nil
}

func printReview(resp *api.ReviewResponse) {
	r, st := resp.Review, resp.Status
	latest := r.Latest()
	fmt.Printf("R%d: %s\n", r.ID, r.Title)
	fmt.Printf("  state:    %s, by %s, target %s\n", r.State, r.Author, r.Target)
	fmt.Printf("  revision: %d of %d, head %s, %d changeset(s)\n", latest.Number, len(r.Revisions), short(latest.Head), len(latest.Stack))
	if latest.Interdiff != "" {
		fmt.Printf("  change since previous revision: %s\n", latest.Interdiff)
	}
	if r.Description != "" {
		fmt.Printf("\n%s\n", indent(r.Description, "  "))
	}
	if st != nil {
		fmt.Printf("\n  overall change: %s\n", st.Class)
		for _, f := range st.Files {
			fmt.Printf("    %-10s %s\n", f.Class, f.Path)
		}
		fmt.Println()
		for _, req := range st.Requirements {
			mark := "✗"
			if req.Satisfied {
				mark = "✓"
			}
			who := "anyone but the author"
			if len(req.Owners) > 0 && !req.Trivial {
				who = strings.Join(req.Owners, ", ")
			}
			fmt.Printf("  %s /%s: %d/%d approval(s) from %s\n", mark, req.Scope, len(req.Have), req.Need, who)
		}
		for _, a := range st.Stale {
			fmt.Printf("  (stale approval by %s on revision %d)\n", a.User, a.Revision)
		}
		if st.Approved {
			fmt.Println("  => approved, ready to land")
		} else {
			for _, reason := range st.Reasons {
				fmt.Printf("  => %s\n", reason)
			}
		}
	}
	if !r.Landed.IsZero() {
		fmt.Printf("  landed as %s\n", short(r.Landed))
	}
	for _, c := range r.Comments {
		loc := ""
		if c.Path != "" {
			loc = fmt.Sprintf(" on %s:%d", c.Path, c.Line)
		}
		fmt.Printf("\n  [%s, r%d, %s]%s\n%s\n", c.Author, c.Revision, time.Unix(c.Time, 0).UTC().Format("2006-01-02 15:04"), loc, indent(c.Body, "    "))
	}
}
