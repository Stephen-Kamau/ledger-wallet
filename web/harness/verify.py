# harness/verify.py — drives the reversal surface in a real browser, signed in, on the
# data seed.mjs put in a real ledgerd. This is the UI verification contract's evidence,
# not a unit test: it asserts nothing, it RENDERS and reports, and the screenshots it
# writes are what a reviewer looks at.
#
#   bash web/harness/up.sh                      # terminal 1 (prints the account ids)
#   OUT=/tmp/shots ALICE1=<alice1 id> python3 web/harness/verify.py
#
# Four paths, each provoked for real rather than mocked:
#   happy      reverse a leg the counterparty account can fund
#   403        reverse a leg that went to Bob — Alice cannot authorise his debit
#   409-class  reverse a leg bigger than what is left to fund the correction
#   409        a second tab reverses first, so this page's Reverse is stale
#
# Desktop widths only (1440x900), per the repo's UI verification contract.

import json, os, sys
from playwright.sync_api import sync_playwright

OUT = os.environ.get("OUT", ".")
A1 = os.environ["ALICE1"]
errs = []

def log(*a): print(*a, flush=True)

with sync_playwright() as pw:
    b = pw.chromium.launch()
    ctx = b.new_context(viewport={"width": 1440, "height": 900})
    ctx.add_init_script("sessionStorage.setItem('ledger.token','alice')")

    def new_page():
        p = ctx.new_page()
        p.on("console", lambda m: errs.append(m.text) if m.type == "error" else None)
        p.on("pageerror", lambda e: errs.append(str(e)))
        return p

    def open_statement(p):
        p.goto("http://localhost:5173/", wait_until="networkidle")
        p.locator(".account-row", has=p.locator(f".acct-id:text-is('{A1}')")).first.click()
        p.wait_for_selector(".statement tbody tr")

    page = new_page()
    open_statement(page)
    shot = lambda n: page.screenshot(path=f"{OUT}/{n}.png", full_page=True)

    shot("01-statement-before")
    log("STEP01 rows=", page.locator(".statement tbody tr").count())
    log("STEP01 reverse_buttons=", page.get_by_role("button", name="Reverse").count())
    log("STEP01 amounts=", json.dumps(page.locator(".statement tbody tr td.num").all_inner_texts()))

    def reverse_row(text):
        r = page.locator(".statement tbody tr").filter(has_text=text).first
        r.get_by_role("button", name="Reverse").click()
        page.get_by_role("button", name="Confirm").click()

    # --- happy path: reverse the -10.99 leg (alice1 -> alice2; alice2 can fund
    # the correction, so this one goes through) -------------------------------
    r1 = page.locator(".statement tbody tr").filter(has_text="-10.99").first
    r1.get_by_role("button", name="Reverse").click()
    page.wait_for_selector(".confirm-q")
    shot("02-confirm")
    log("STEP02 confirm=", page.locator(".confirm-q").first.inner_text())

    page.get_by_role("button", name="Confirm").click()
    page.wait_for_selector(".notice-banner", timeout=15000)
    page.wait_for_timeout(800)
    shot("03-reversed")
    log("STEP03 notice=", page.locator(".notice-banner").inner_text())
    log("STEP03 badges=", json.dumps(page.locator(".rev-badge").all_inner_texts()))
    log("STEP03 captions=", json.dumps(page.locator(".rev-caption td:nth-child(2)").all_inner_texts()))
    log("STEP03 reverse_buttons=", page.get_by_role("button", name="Reverse").count())
    log("STEP03 reversed_rows=", page.locator("tr.reversed-row").count(),
        "reversal_rows=", page.locator("tr.reversal-row").count())

    # --- 403 not-your-account -------------------------------------------------
    # The -15,000 leg went to Bob. Reversing it has to debit Bob's account, which
    # Alice cannot authorise, so the ledger itself answers 403.
    reverse_row("-150.00")
    page.wait_for_selector(".rev-error", timeout=15000)
    shot("04-403-not-your-account")
    log("STEP04 row_error=", page.locator(".rev-error .field-error").inner_text())
    log("STEP04 offered=", json.dumps(page.locator(".rev-error button").all_inner_texts()))
    page.get_by_role("button", name="Dismiss").click()

    # --- 409-class: the correction would overdraw the account it must come from.
    # The +2,500.00 mint credit is bigger than what is left in the account.
    reverse_row("2,500.00")
    page.wait_for_selector(".rev-error", timeout=15000)
    shot("04b-insufficient-funds")
    log("STEP04b row_error=", page.locator(".rev-error .field-error").inner_text())
    page.get_by_role("button", name="Dismiss").click()

    # --- 409 already-reversed -------------------------------------------------
    # The real-world route: a second tab reverses an entry while this page's
    # statement is stale, so this page still offers Reverse on it.
    other = new_page()
    open_statement(other)
    orow = other.locator(".statement tbody tr").filter(has_text="75.00").first
    orow.get_by_role("button", name="Reverse").click()
    other.get_by_role("button", name="Confirm").click()
    other.wait_for_selector(".notice-banner", timeout=15000)
    other.close()

    reverse_row("75.00")
    page.wait_for_selector(".rev-error", timeout=15000)
    shot("05-409-already-reversed")
    log("STEP05 row_error=", page.locator(".rev-error .field-error").inner_text())
    log("STEP05 offered=", json.dumps(page.locator(".rev-error button").all_inner_texts()))

    page.get_by_role("button", name="Refresh statement").click()
    page.wait_for_timeout(1000)
    shot("06-after-refresh")
    log("STEP06 badges=", json.dumps(page.locator(".rev-badge").all_inner_texts()))
    log("STEP06 captions=", json.dumps(page.locator(".rev-caption td:nth-child(2)").all_inner_texts()))

    page.close()
    b.close()

open(f"{OUT}/console-errors.txt", "w").write("\n".join(errs))
log("CONSOLE_ERRORS=", len(errs))
