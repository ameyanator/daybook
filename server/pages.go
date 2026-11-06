package main

import "net/http"

// Public web pages: the home page, plus the privacy policy and account
// deletion pages that Google sign-in and the app stores link to.

const contactEmail = "ameya.business26@gmail.com"

func registerPages(mux *http.ServeMux) {
	for path, body := range map[string]string{
		"GET /{$}":            homePage,
		"GET /privacy":        privacyPage,
		"GET /delete-account": deleteAccountPage,
	} {
		html := pageShell(body)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(html))
		})
	}
}

func pageShell(body string) string {
	return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Daybook</title>
<style>
body{font-family:-apple-system,system-ui,sans-serif;background:#FAF8F5;color:#1F1B16;
max-width:40rem;margin:3rem auto;padding:0 1rem;line-height:1.55}
h1{font-size:2rem;margin-bottom:.2rem}h2{font-size:1.15rem;margin-top:2rem}
p,li{color:#3d3832}a{color:#C2552B}.muted{color:#8A8178;font-size:.9rem}
footer{margin-top:3rem;border-top:1px solid #E6E0D9;padding-top:1rem;font-size:.9rem}
</style></head><body>` + body + `
<footer><a href="/">Daybook</a> · <a href="/privacy">Privacy policy</a> · <a href="/delete-account">Delete your account</a> · <a href="mailto:` + contactEmail + `">Contact</a></footer>
</body></html>`
}

const homePage = `<h1>Daybook</h1>
<p>Your day, one line at a time.</p>
<p>Daybook is a simple schedule: pick a date (and a time, or "all day"), write one line about what happens,
and see everything in order. You can share your calendar, read-only, with people you choose.</p>`

const privacyPage = `<h1>Privacy policy</h1>
<p class="muted">Last updated: November 6, 2026</p>

<p>Daybook ("the app") is a personal schedule app. This policy explains what data it handles and why.
Questions: <a href="mailto:` + contactEmail + `">` + contactEmail + `</a>.</p>

<h2>What we collect</h2>
<ul>
<li><b>Account information</b>: when you sign in with Google or Apple, we receive your name, your email
address, and an account identifier from that provider. We never see or store your password.</li>
<li><b>Your entries</b>: the one-line descriptions, dates and times you add.</li>
<li><b>Sharing information</b>: whom you've shared your calendar with, and invite links you create.</li>
</ul>
<p>We don't collect location, contacts, photos, device identifiers for advertising, or analytics.
There are no ads and no third-party tracking in the app.</p>

<h2>How we use it</h2>
<p>Only to run the app: to sign you in, store and show your schedule, and let the people you choose see it.
We don't sell your data or use it for advertising.</p>

<h2>Who can see your data</h2>
<ul>
<li><b>You</b>.</li>
<li><b>People you share with</b>: they can see your entries (read-only) and your name and email address.
You can stop sharing at any time in the app.</li>
<li><b>Service providers that host the app</b>: Render (servers) and Neon (database), which store data on our
behalf. Google and Apple handle sign-in.</li>
</ul>

<h2>Security</h2>
<p>Data is sent over encrypted connections (HTTPS). Sign-in sessions and invite links are stored only as
one-way hashes.</p>

<h2>Keeping and deleting your data</h2>
<p>We keep your data while you have an account. You can delete your account at any time in the app
(<b>Share → Delete account</b>). This permanently removes your account, all your entries, and all your sharing.
See <a href="/delete-account">Delete your account</a> for other options.</p>

<h2>Children</h2>
<p>Daybook isn't directed at children under 13, and we don't knowingly collect their data.</p>

<h2>Changes</h2>
<p>If this policy changes, we'll update this page and the date above.</p>`

const deleteAccountPage = `<h1>Delete your Daybook account</h1>
<p><b>In the app:</b> open Daybook, tap <b>Share</b>, scroll to <b>Account</b>, and tap <b>Delete account</b>.</p>
<p><b>Without the app:</b> email <a href="mailto:` + contactEmail + `?subject=Delete%20my%20Daybook%20account">` + contactEmail + `</a>
from the email address you signed in with, asking us to delete your account. We'll do it within 7 days.</p>
<h2>What gets deleted</h2>
<p>Everything: your account (name and email), all your entries, everyone you shared with, calendars shared
with you, and any unused invite links. Deletion is permanent, and nothing is kept afterwards.</p>`
