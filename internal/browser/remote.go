package browser

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// This file is the sign-in through a browser the caller does not start: a
// shared browser service reached over its DevTools WebSocket endpoint.
//
// What it guarantees, and how:
//
//   - Isolation. The sign-in runs in a browser context created for this one
//     call (Target.createBrowserContext, the protocol's incognito profile),
//     never in the browser's default context. The context is created with
//     disposeOnDetach, so the browser itself discards it when this
//     connection goes away, whatever happens to this process.
//   - Disposal. On every way out (success, failure, timeout, cancellation,
//     panic) the context is disposed explicitly and the connection closed.
//     Only that context and the one page in it are ever acted on: the
//     browser is never closed, and no other context or target is disposed,
//     closed, attached to or changed.
//   - Secrecy. The endpoint may carry a token. Every error is built here
//     from fixed text plus the endpoint reduced to scheme://host[:port];
//     text coming back from the dialer is scrubbed, and the underlying
//     error is not kept in the chain. Credentials, cookies and page content
//     are never logged or put in an error.
//   - One submission. The sign-in form is submitted at most once per call,
//     and an error says whether that can have happened yet. The line is
//     drawn before the first keystroke into the password field, not at the
//     click: a page can submit itself while it is being filled, and this
//     code cannot rule that out for a page it does not control.
//   - Only the portal. The credentials are typed and submitted only while
//     the page is on the configured portal's origin, and only cookies that
//     belong to the portal's host are returned.
//   - No guessed success. A sign-in is reported as successful only on
//     positive evidence that the browser left the sign-in form.
//   - Bounded. Timeout covers the dial, the page work and the disposal.
//
// Known limit: the size of a message the remote browser sends is not
// bounded. chromedp dials the endpoint itself and offers neither a read
// limit nor a way to supply the connection.

// Sentinels reported (through errors.Is) by errors of the remote sign-in.
// The root package re-exports them.
var (
	// ErrInvalidOptions: the options cannot be used together, or the
	// endpoint is not a usable ws:// or wss:// URL. Nothing was sent.
	ErrInvalidOptions = errors.New("invalid authentication options")

	// ErrBrowserConnection: the remote browser could not be reached, or
	// the connection to it was lost.
	ErrBrowserConnection = errors.New("remote browser connection failed")

	// ErrNotSubmitted: the sign-in stopped before the password was typed
	// into the page, so the form cannot have been submitted. Never
	// reported together with ErrInvalidOptions.
	ErrNotSubmitted = errors.New("credentials were not submitted")

	// ErrSubmitted: the sign-in stopped after the password had started to
	// be typed into the page. The portal may have seen the credentials;
	// the outcome is unknown unless ErrRejected is reported too.
	ErrSubmitted = errors.New("credentials may have been submitted")

	// ErrOrigin: the page the credentials were about to be typed into, or
	// submitted from, was not on the portal's origin.
	ErrOrigin = errors.New("sign-in page is not on the portal's origin")

	// ErrRejected: the portal answered the submission with its
	// sign-in error.
	ErrRejected = errors.New("sign-in rejected by the portal")
)

// Stage says how far a remote sign-in got.
type Stage int

const (
	// StageOptions: rejected before anything was sent anywhere.
	StageOptions Stage = iota
	// StageConnect: connecting to the remote browser.
	StageConnect
	// StageBeforeSubmit: the password had not been typed into the page;
	// the form cannot have been submitted.
	StageBeforeSubmit
	// StageAfterSubmit: the password was being or had been typed into
	// the page; the form may have been submitted.
	StageAfterSubmit
)

// SignInError is every error the remote sign-in returns, and the error of
// the checks the local sign-in shares with it. Its text is built only from
// fixed strings, origins (scheme, host and port) and the redacted endpoint.
type SignInError struct {
	// Stage says how far the sign-in got.
	Stage Stage
	// Connection is set when the browser could not be reached or the
	// connection to it was lost.
	Connection bool
	// Rejected is set when the portal showed its sign-in error.
	Rejected bool
	// Origin is set when the page was not on the portal's origin.
	Origin bool

	local  bool // raised by the local flow
	msg    string
	ctxErr error
}

// Error implements the error interface.
func (e *SignInError) Error() string {
	var stage string
	switch e.Stage {
	case StageOptions:
		stage = "nothing was sent"
	case StageAfterSubmit:
		stage = "credentials may have been submitted; outcome unknown"
		if e.Rejected {
			stage = "credentials were submitted once"
		}
	default:
		stage = "credentials were not submitted"
	}
	prefix := "remote browser sign-in: "
	if e.local {
		prefix = "browser sign-in: "
	}
	return prefix + e.msg + " (" + stage + ")"
}

// Is reports the sentinels that describe this error.
func (e *SignInError) Is(target error) bool {
	switch target {
	case ErrInvalidOptions:
		return e.Stage == StageOptions
	case ErrBrowserConnection:
		return e.Connection
	case ErrNotSubmitted:
		// Not for StageOptions: a caller that retries on "not
		// submitted" must not retry options that can never work.
		return e.Stage == StageConnect || e.Stage == StageBeforeSubmit
	case ErrSubmitted:
		return e.Stage == StageAfterSubmit
	case ErrRejected:
		return e.Rejected
	case ErrOrigin:
		return e.Origin
	}
	return false
}

// Unwrap returns context.DeadlineExceeded or context.Canceled when one of
// them ended the sign-in, and nothing else: errors from the dialer and the
// protocol are deliberately not kept in the chain.
func (e *SignInError) Unwrap() error {
	return e.ctxErr
}

// stepError is a failure whose text is fixed here and safe to report.
type stepError string

func (e stepError) Error() string { return string(e) }

// The waits of the remote flow. The settle waits are those of the local
// flow.
const (
	remoteSettleAfterLoad     = 1 * time.Second
	remoteSettleAfterFill     = 500 * time.Millisecond
	remoteSettleAfterAccepted = 1 * time.Second
	remoteMaxTeardown         = 5 * time.Second
	remotePollInterval        = 100 * time.Millisecond
	remoteOutcomePollInterval = 250 * time.Millisecond
)

// How long the remote flow looks for a form field or the submit button
// before giving up. Variables so tests can shorten them.
var (
	remoteFieldWait  = 10 * time.Second
	remoteButtonWait = 5 * time.Second
)

// remoteTestHook, when set by a test, is called at named points of the
// remote flow. It is nil outside tests.
var remoteTestHook func(point string)

func discardf(string, ...any) {}

// teardownBudget is the part of the overall timeout held back for disposing
// the browser context.
func teardownBudget(timeout time.Duration) time.Duration {
	b := timeout / 4
	if b > remoteMaxTeardown {
		b = remoteMaxTeardown
	}
	return b
}

// authenticateRemote signs in through the browser at opts.BrowserURL.
func authenticateRemote(ctx context.Context, opts *AuthenticateOptions) (_ *AuthResult, retErr error) {
	if err := validateRemoteOptions(opts); err != nil {
		return nil, err
	}
	red := newRedactor(opts.BrowserURL)
	wantOrigin := canonicalOrigin(opts.BaseURL)
	if wantOrigin == "" {
		return nil, &SignInError{Stage: StageOptions, msg: "the portal's base URL is not an http:// or https:// URL with a plain ASCII host"}
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultAuthenticateOptions().Timeout
	}
	deadline := time.Now().Add(timeout)
	workDeadline := deadline.Add(-teardownBudget(timeout))

	if err := ctx.Err(); err != nil {
		return nil, &SignInError{Stage: StageConnect, msg: "the context was done before connecting", ctxErr: err}
	}

	// The browser context must outlive the caller's cancellation long
	// enough to be disposed over the same connection, so the chromedp
	// contexts do not descend from ctx's cancellation. Work is done under
	// contexts made by bounded, which carry both the caller's cancellation
	// and the deadline for the work.
	bounded := func(parent context.Context) (context.Context, context.CancelFunc) {
		bctx, cancel := context.WithDeadline(parent, workDeadline)
		stop := context.AfterFunc(ctx, cancel)
		return bctx, func() { stop(); cancel() }
	}

	// connCtx owns the connection and nothing else: it never gets a page.
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.WithoutCancel(ctx), opts.BrowserURL, chromedp.NoModifyURL)
	connCtx, connCancel := chromedp.NewContext(allocCtx)
	conn := chromedp.FromContext(connCtx)
	setupCtx, setupCancel := bounded(connCtx)

	s := &remoteSession{conn: conn, connCancel: connCancel, allocCancel: allocCancel}
	var result *AuthResult
	defer func() {
		// Runs on every way out, a panic included.
		setupCancel()
		if s.teardown(deadline) {
			return
		}
		var re *SignInError
		if errors.As(retErr, &re) {
			re.msg += "; disposal of the browser context was not confirmed"
		} else if result != nil {
			result.ContextDisposalUnconfirmed = true
		}
	}()

	// Connect. The allocator is asked directly, with connCtx (not a
	// bounded context) as its context: chromedp closes the connection as
	// soon as the context given here is done, and the connection has to
	// survive the end of the work to dispose the browser context. Nothing
	// chromedp would log is let through: its messages can quote protocol
	// traffic.
	type allocated struct {
		b   *chromedp.Browser
		err error
	}
	allocCh := make(chan allocated, 1)
	go func() {
		b, err := conn.Allocator.Allocate(connCtx,
			chromedp.WithDialTimeout(time.Until(workDeadline)),
			chromedp.WithBrowserLogf(discardf),
			chromedp.WithBrowserErrorf(discardf),
		)
		allocCh <- allocated{b, err}
	}()
	var a allocated
	select {
	case a = <-allocCh:
	case <-setupCtx.Done():
		connCancel() // aborts the dial; nothing exists remotely yet
		a = <-allocCh
		if a.err == nil {
			a.err = setupCtx.Err()
		}
	}
	if a.err != nil {
		e := &SignInError{Stage: StageConnect, Connection: true}
		switch {
		case ctx.Err() != nil:
			e.msg = "cancelled while connecting to the remote browser at " + red.display
			e.ctxErr = ctx.Err()
		case errors.Is(setupCtx.Err(), context.DeadlineExceeded) || errors.Is(a.err, context.DeadlineExceeded):
			e.msg = "timed out connecting to the remote browser at " + red.display
			e.ctxErr = context.DeadlineExceeded
		default:
			e.msg = "could not connect to the remote browser at " + red.display + ": " + red.scrub(a.err.Error())
		}
		return nil, e
	}
	conn.Browser = a.b

	var (
		submitted atomic.Bool
		step      = "opening an isolated browser context"
		cookies   []*network.Cookie
		workCtx   = setupCtx
	)
	fail := func(err error) error {
		return classifyRemote(err, step, submitted.Load(), ctx, workCtx, a.b, red)
	}
	named := func(name string, action chromedp.Action) chromedp.Action {
		return chromedp.ActionFunc(func(ctx context.Context) error {
			step = name
			if opts.DebugLog {
				fmt.Printf("[chromedp] remote: %s\n", name)
			}
			return action.Do(ctx)
		})
	}

	// A browser context of this sign-in's own. disposeOnDetach makes the
	// browser discard it when this connection closes, whatever happens to
	// this process in between.
	onBrowser := cdp.WithExecutor(setupCtx, a.b)
	contextID, err := target.CreateBrowserContext().WithDisposeOnDetach(true).Do(onBrowser)
	if err != nil {
		return nil, fail(err)
	}
	s.contextID = contextID

	// One page, inside that context.
	targetID, err := createPage(onBrowser, contextID)
	if err != nil {
		return nil, fail(err)
	}
	pageCtx, pageCancel := chromedp.NewContext(connCtx, chromedp.WithTargetID(targetID))
	s.pageCancel = pageCancel
	pageWorkCtx, pageWorkCancel := bounded(pageCtx)
	defer pageWorkCancel()
	workCtx = pageWorkCtx

	// Attaches to the page.
	if err := chromedp.Run(workCtx); err != nil {
		return nil, fail(err)
	}
	if remoteTestHook != nil {
		remoteTestHook("context-open")
	}

	loginURL := opts.BaseURL + "/public/"
	var before pageState
	actions := []chromedp.Action{
		named("opening the sign-in page", chromedp.Navigate(loginURL)),
		chromedp.Sleep(remoteSettleAfterLoad),
		named("filling the username field", chromedp.ActionFunc(func(ctx context.Context) error {
			sel, err := firstPresent(ctx, usernameSelectors, remoteFieldWait)
			if err != nil {
				return err
			}
			if sel == "" {
				return stepError("the username field was not found")
			}
			if err := requirePortalOrigin(ctx, wantOrigin); err != nil {
				return err
			}
			return chromedp.SendKeys(sel, opts.Username, chromedp.ByQuery).Do(ctx)
		})),
		named("filling the password field", chromedp.ActionFunc(func(ctx context.Context) error {
			sel, err := firstPresent(ctx, passwordSelectors, remoteFieldWait)
			if err != nil {
				return err
			}
			if sel == "" {
				return stepError("the password field was not found")
			}
			if err := requirePortalOrigin(ctx, wantOrigin); err != nil {
				return err
			}
			// The page as it is before the password goes in: what the
			// portal's answer is told apart from, and what tells
			// whether the page went on to submit itself.
			st, err := readPageState(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return stepError("the sign-in page could not be read before filling the password")
			}
			before = st
			var installed bool
			if err := chromedp.Evaluate(watchSubmissionJS, &installed).Do(ctx); err != nil && ctx.Err() != nil {
				return ctx.Err()
			}
			// From the first keystroke of the password on, the page
			// holds what it needs to submit itself (a handler on the
			// field, a script, a lost reply to the click). Anything
			// that goes wrong from here is "may have been submitted".
			submitted.Store(true)
			return chromedp.SendKeys(sel, opts.Password, chromedp.ByQuery).Do(ctx)
		})),
		chromedp.Sleep(remoteSettleAfterFill),
		named("submitting the sign-in form", chromedp.ActionFunc(func(ctx context.Context) error {
			// A page that submitted itself while it was being filled
			// is not submitted again: one submission per call. Its
			// answer is waited for like any other.
			st, err := readPageState(ctx)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if alreadySubmitted(st, err, before) {
				return nil
			}
			return submitOnce(ctx, pageSubmitDriver{wantOrigin: wantOrigin}, func() { submitted.Store(true) })
		})),
		named("waiting for the portal's answer", chromedp.ActionFunc(func(ctx context.Context) error {
			// There is no "probably fine": this returns on positive
			// evidence either way, or not at all. A failed evaluation
			// (a navigation in flight) is not evidence.
			for {
				if st, err := readPageState(ctx); err == nil {
					switch judgeOutcome(st, before, wantOrigin) {
					case outcomeAccepted:
						return nil
					case outcomeRejected:
						return ErrRejected
					}
				}
				t := time.NewTimer(remoteOutcomePollInterval)
				select {
				case <-ctx.Done():
					t.Stop()
					return ctx.Err()
				case <-t.C:
				}
			}
		})),
		chromedp.Sleep(remoteSettleAfterAccepted),
		named("reading the session cookies", chromedp.ActionFunc(func(ctx context.Context) error {
			got, err := readPortalCookies(ctx, opts.BaseURL, wantOrigin)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return stepError("the session cookies could not be read")
			}
			cookies = got
			return nil
		})),
	}
	if opts.UserAgent != "" {
		ua := named("setting the user agent", emulation.SetUserAgentOverride(opts.UserAgent))
		actions = append([]chromedp.Action{ua}, actions...)
	}

	if err := chromedp.Run(workCtx, actions...); err != nil {
		return nil, fail(err)
	}

	result = buildAuthResult(cookies)
	return result, nil
}

// classifyRemote turns a failure of the page work into a SignInError.
func classifyRemote(err error, step string, submitted bool, callerCtx, workCtx context.Context, b *chromedp.Browser, red *redactor) *SignInError {
	e := &SignInError{Stage: StageBeforeSubmit}
	if submitted {
		e.Stage = StageAfterSubmit
	}

	lost := false
	select {
	case <-b.LostConnection:
		lost = true
	default:
	}

	var se stepError
	var pe *cdproto.Error
	var om *originMismatch
	switch {
	case errors.Is(err, ErrRejected):
		e.Rejected = true
		e.msg = "the portal rejected the sign-in"
	case errors.As(err, &om):
		e.Origin = true
		e.msg = om.Error()
	case lost:
		e.Connection = true
		e.msg = "the connection to the remote browser at " + red.display + " was lost while " + step
	case callerCtx.Err() != nil:
		e.ctxErr = callerCtx.Err()
		e.msg = "cancelled while " + step
	case errors.Is(workCtx.Err(), context.DeadlineExceeded):
		e.ctxErr = context.DeadlineExceeded
		e.msg = "timed out while " + step
	case errors.As(err, &se):
		e.msg = string(se)
	case errors.As(err, &pe):
		// A DevTools protocol error. Only its numeric code is
		// repeated: the message is whatever the remote end chose to
		// send, and by now it may know the password.
		e.msg = "the browser refused a command (protocol error " + strconv.FormatInt(pe.Code, 10) + ") while " + step
	default:
		e.msg = "failed while " + step
		if code := netErrorCode(err); code != "" {
			e.msg += ": " + code
		}
	}
	return e
}

// netErrorCode returns the browser's network error code (such as
// "net::ERR_NAME_NOT_RESOLVED") when err is a failed page load, and ""
// otherwise. Only the code itself is returned, never the rest of the text.
func netErrorCode(err error) string {
	const marker = "net::ERR_"
	text := err.Error()
	i := strings.Index(text, marker)
	if i < 0 {
		return ""
	}
	end := i + len(marker)
	for end < len(text) && (text[end] == '_' || (text[end] >= 'A' && text[end] <= 'Z') || (text[end] >= '0' && text[end] <= '9')) {
		end++
	}
	return text[i:end]
}

// createPage creates the sign-in's page in its browser context. A browser
// with windows (anything but the headless shell) has no window for a new
// browser context yet and refuses to open a tab in one; it is then asked
// for a window of that context instead. Either way the page belongs to the
// context and goes away with it.
func createPage(onBrowser context.Context, contextID cdp.BrowserContextID) (target.ID, error) {
	id, err := target.CreateTarget("about:blank").WithBrowserContextID(contextID).Do(onBrowser)
	if needsWindow(err) {
		id, err = target.CreateTarget("about:blank").WithBrowserContextID(contextID).WithNewWindow(true).Do(onBrowser)
	}
	return id, err
}

// needsWindow reports whether err is the browser's refusal to open a tab
// in a browser context that has no window. Nothing else is retried.
func needsWindow(err error) bool {
	var pe *cdproto.Error
	return err != nil && errors.As(err, &pe) &&
		pe.Code == -32000 && strings.Contains(pe.Message, "no browser is open")
}

// remoteSession is what one remote sign-in holds in the remote browser.
type remoteSession struct {
	conn        *chromedp.Context
	connCancel  context.CancelFunc
	allocCancel context.CancelFunc
	pageCancel  context.CancelFunc // nil until the page exists

	contextID cdp.BrowserContextID // "" until the browser context exists
}

// teardown disposes the browser context of this sign-in and closes the
// connection, returning by deadline. It reports whether the disposal is
// known to have happened: confirmed by the browser, or there was nothing to
// dispose, or the connection is already gone (the context was created with
// disposeOnDetach).
func (s *remoteSession) teardown(deadline time.Time) bool {
	confirmed := true
	if b := s.conn.Browser; b != nil && s.contextID != "" {
		until := time.Now().Add(remoteMaxTeardown)
		if until.After(deadline) {
			until = deadline
		}
		confirmed = disposeBrowserContext(b, s.contextID, until)
	}

	// Cancelling makes chromedp detach from the page (closing it, if the
	// disposal above did not) and then close the websocket, which is what
	// triggers disposeOnDetach should the explicit disposal not have gone
	// through. chromedp bounds its own part to one second; do not wait for
	// it beyond the deadline.
	done := make(chan struct{})
	go func() {
		defer close(done)
		if s.pageCancel != nil {
			s.pageCancel()
		}
		s.connCancel()
		s.allocCancel()
		s.conn.Allocator.Wait()
	}()
	wait := time.NewTimer(time.Until(deadline))
	defer wait.Stop()
	select {
	case <-done:
	case <-wait.C:
	}
	return confirmed
}

// disposeBrowserContext asks the browser to dispose one browser context and
// reports whether it is known to be gone.
func disposeBrowserContext(b *chromedp.Browser, id cdp.BrowserContextID, until time.Time) bool {
	select {
	case <-b.LostConnection:
		// The connection is gone, and with it the context.
		return true
	default:
	}
	tctx, cancel := context.WithDeadline(context.Background(), until)
	defer cancel()
	go func() {
		select {
		case <-b.LostConnection:
			cancel()
		case <-tctx.Done():
		}
	}()
	err := target.DisposeBrowserContext(id).Do(cdp.WithExecutor(tctx, b))
	if err == nil {
		return true
	}
	select {
	case <-b.LostConnection:
		return true
	default:
		return false
	}
}

// firstPresent returns the first of selectors that matches an element of the
// page, looking again until one does or wait has passed. It returns "" when
// none matched in time, and an error only when ctx is done.
func firstPresent(ctx context.Context, selectors []string, wait time.Duration) (string, error) {
	giveUp := time.Now().Add(wait)
	for {
		for _, sel := range selectors {
			var present bool
			js := "document.querySelector(" + strconv.Quote(sel) + ") !== null"
			if err := chromedp.Evaluate(js, &present).Do(ctx); err == nil && present {
				return sel, nil
			}
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		if !time.Now().Before(giveUp) {
			return "", nil
		}
		t := time.NewTimer(remotePollInterval)
		select {
		case <-ctx.Done():
			t.Stop()
			return "", ctx.Err()
		case <-t.C:
		}
	}
}

// submitDriver is what submitOnce needs from the page.
type submitDriver interface {
	// find returns the submit control to use, or "" when there is none.
	find(ctx context.Context) (string, error)
	// guard is the last check before anything that submits is sent.
	guard(ctx context.Context) error
	// ready waits until the control can be clicked. It sends nothing.
	ready(ctx context.Context, sel string) error
	// click clicks the control once.
	click(ctx context.Context, sel string) error
	// enter presses Enter in the password field once.
	enter(ctx context.Context) error
}

// submitOnce submits the sign-in form at most once. mark is called
// immediately before the one thing that can submit is sent; whatever happens
// after that, nothing else is tried. A locked account is the cost of a
// second submission, so a failed click is never followed by another control
// or by the Enter key.
func submitOnce(ctx context.Context, d submitDriver, mark func()) error {
	sel, err := d.find(ctx)
	if err != nil {
		return err
	}
	if sel != "" {
		if err := d.ready(ctx, sel); err != nil {
			return err
		}
	}
	if err := d.guard(ctx); err != nil {
		return err
	}
	mark()
	if sel == "" {
		return d.enter(ctx)
	}
	return d.click(ctx, sel)
}

// pageSubmitDriver drives the page of the context it is given.
type pageSubmitDriver struct {
	wantOrigin string
}

func (d pageSubmitDriver) guard(ctx context.Context) error {
	return requirePortalOrigin(ctx, d.wantOrigin)
}

func (pageSubmitDriver) find(ctx context.Context) (string, error) {
	return firstPresent(ctx, submitSelectors, remoteButtonWait)
}

func (pageSubmitDriver) ready(ctx context.Context, sel string) error {
	return chromedp.WaitVisible(sel, chromedp.ByQuery).Do(ctx)
}

func (pageSubmitDriver) click(ctx context.Context, sel string) error {
	return chromedp.Click(sel, chromedp.NodeVisible, chromedp.ByQuery).Do(ctx)
}

func (pageSubmitDriver) enter(ctx context.Context) error {
	return chromedp.SendKeys(`input[type="password"]`, "\n", chromedp.ByQuery).Do(ctx)
}
