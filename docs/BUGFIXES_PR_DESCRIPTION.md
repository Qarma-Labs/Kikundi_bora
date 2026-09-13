# PR: Five bug fixes — loan flow, social-fund flow, member-approval flow

Full-stack fixes, one commit-series per bug. All five touch backend + frontend together.

---

## BUG 1 — Full member detail visible before approval (small)

**Finding:** the backend already returned full member records (`GET /api/v1/members?status=pending` has no projection — `handlers/members.go:List`); the approval queue UI was only rendering a subset.

**Fix (frontend, `src/routes/wanachama-kusubiri.tsx`):** the katibu approval queue now renders **everything captured at registration** before the Idhinisha/Kataa actions: photo (already), name, member_no, phone, gender, occupation, email, **address**, **joined_at**, next-of-kin, registrar audit line. No backend change needed (verified).

---

## BUG 2 (big) — Loan approval requests now routed through the full chain

**Documented findings (state machine BEFORE the fix):**

The multi-role chain **was already designed and coded** — the Loan model has 8 sequential-trail columns and `handlers/leadership.go ApproveLoan` implemented **Hazina → Katibu → Bodi → Mwenyekiti** while status stays `PENDING`. What was broken:

1. The page/queue routes were locked behind `RequireLeadership` — appointed bodi members (`users.role = member`) could never reach their stage.
2. No "your turn" concept: every leader saw every loan with no stage indicator; no notifications ever pinged the next approver, so loans sat silently.
3. **Three competing paths to APPROVED** undercut the chain:
   - legacy `POST /loans/:id/approve|reject` (`RequireRoles(Chair, Treasurer)`) — direct bypass skipping katibu + bodi entirely
   - committee unanimous review auto-finalized straight to `APPROVED` without the trail
   - the intended sequential path

**Backend fix:**
- **Removed the legacy bypass routes entirely** (no frontend callers existed).
- Committee unanimous APPROVE no longer finalizes: it now satisfies the **bodi stage** (`bodi_approved_*` set) and returns the loan to `PENDING`; finalization stays with Mwenyekiti. Committee REJECT still kills the loan immediately (unchanged).
- `GET /uongozi/mikopo/pending` now computes per loan `awaiting_role` ("hazina"|"katibu"|"bodi"|"mwenyekiti") and `my_turn` for the caller; supports `?my_turn=true` for each role's "loans awaiting MY action" queue.
- Chain routes re-guarded with `RequireLoanCommitteeMember()` (leadership **or** appointed bodi members) and moved outside the leadership group whose `RequireLeadership` middleware blocked bodi members.
- **Stage handoff notifications**: each approval now pings the next stage's role (hazina → katibu → bodi → mwenyekiti → applicant + treasurer at finalization). Also fixed the latent broken committee-notification query in Apply.
- Status enum unchanged: `PENDING | UNDER_REVIEW | APPROVED | OUTSTANDING | CLOSED | REJECTED` — stage tracking stays on the timestamp trail (as already documented in `docs/SECURITY_AUDIT.md`).

**Frontend fix:**
- `/uongozi/mikopo` now admits bodi members (was "Huna ruhusa") and shows a **"Zamu yako"** chip on loans at the viewer's stage; stage-appropriate actions: Idhinisha per stage, **Toa Fedha** only for hazina at the APPROVED stage (existing), loan review (Kataa) lives on the committee page.
- **New dashboard section** (`LoanApprovalsCard`): "Mikopo Inayosubiri Idhini" with the count of loans at the viewer's stage, rendered on dashibodi for katibu, mweka hazina, mwenyekiti and bodi members — this is the missing section from the bug report.

**Tests (`handlers/bugfix_test.go TestLoanChainHandoff`):** loan appears in hazina's `?my_turn=true` queue only → hazina approves → appears in katibu's queue and NOT hazina's → katibu → bodi → mwenyekiti → `APPROVED` → legacy endpoints 404 → disburse → `OUTSTANDING` → borrower confirmation. Plus the updated lifecycle tests (`TestLoanLifecycleHTTP`, `TestNegativeScenarios`, `TestCommitteeReviewFlow`) now walk the real chain.

---

## BUG 3 — Loan-board (committee) appointment is mwenyekiti's action only

**Backend:** `POST /loan-committee/members` was `RequireRoles(Chair, Secretary)` → now **Chair only** (`main.go`).
**Frontend:** the "Unda Bodi Ya Mikopo" button on `/uongozi/mikopo` moved from katibu's view to mwenyekiti's (`isKatibu` → `isMwenyekiti`).
**Test:** `TestCommitteeAppointChairOnly` — katibu 403, mwenyekiti succeeds.

---

## BUG 4 — Welfare beneficiary confirms receipt ("Nimepokea")

**Finding:** the model already had `member_id` (beneficiary), `received_at`/`received_by` and a confirm endpoint — but leadership could confirm on the beneficiary's behalf, and the beneficiary had no inline action.

**Backend (`handlers/welfare.go ConfirmReceipt`):** restricted to **the beneficiary member themselves only** — the leadership bypass was removed (a witnessed handover doesn't substitute for the recipient's acknowledgement). 403 for anyone else, including leadership; 409 on double-confirm.

**Frontend:**
- The beneficiary's dashboard banner now offers an **inline "Nimepokea"** button (PATCHes `/welfare/events/:id/confirm-receipt`) instead of just linking away.
- `/mfuko-kijamii` dialog confirm action restricted to the beneficiary; the 5-step event timeline already shows the received step to leadership (closed-loop visibility).

**Tests:** `TestWelfareReceiptBeneficiaryOnly` (new) + `TestWelfareConfirmReceipt` / `TestWelfareDisbursePostsLedgerAndReceipt` updated to the beneficiary-only rule — leadership confirm → 403; beneficiary → 200; double → 409.

---

## BUG 5 — Loan borrower confirms receipt after disbursement

**Backend:**
- `models.Loan`: new `BorrowerConfirmedAt *time.Time` — a distinct final state from disbursement (`OUTSTANDING` = hazina paid out; `borrower_confirmed_at` = borrower acknowledges).
- New `PATCH /api/v1/loans/:id/confirm-received` — borrower-only (the authenticated user must own the loan's member row); 403 for anyone else including leadership; 409 on double-confirm; audits + notifies hazina.

**Frontend (`/mikopo` — Mikopo Yangu):**
- `OUTSTANDING` + unconfirmed → chip **"Imetolewa — Inasubiri Uthibitisho Wako"** + button **"Thibitisha Umepokea Mkopo"**.
- Confirmed → chip **"Imetolewa — Imethibitishwa"**, no button.

**Tests:** borrower-confirm covered in `TestLoanChainHandoff` (chair 403, borrower 200, double 409) + FE tests in `loan-bugfix.test.tsx`.

---

## Test summary

- **Backend:** `go vet` clean; new tests `TestLoanChainHandoff`, `TestCommitteeAppointChairOnly`, `TestWelfareReceiptBeneficiaryOnly` all pass; full suite green except the pre-existing `TestFiberUUIDBug` ordering flake (fails on unmodified code too, passes standalone). Bonus: this series **fixes the previously-failing** `TestCommitteeReviewFlow` (it was missing treasury funding).
- **Frontend:** 169/169 tests pass (10 new bug-fix tests); `tsc` error count unchanged from before the fix (all pre-existing in untouched files).

## Breaking-change note

`POST /api/v1/loans/:id/approve` and `POST /api/v1/loans/:id/reject` no longer exist. All approval/rejection goes through the sequential chain (`/uongozi/mikopo/:id/approve`) and committee review (`/loan-committee/loans/:id/review`). The FE `loansApi.approve/reject` methods and their hooks were removed (they had zero call sites).
