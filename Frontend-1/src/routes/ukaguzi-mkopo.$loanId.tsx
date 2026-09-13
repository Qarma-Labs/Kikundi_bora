import { createFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import { AppShell } from "@/components/AppShell";
import { blockAdminFromPage, requireAuth } from "@/lib/role-guards";
import { useAuth } from "@/lib/auth-provider";
import { useCommitteeLoanDetail, useSubmitLoanReview } from "@/hooks/use-loan-committee";
import { useIsCommitteeMember } from "@/hooks/use-loan-committee";
import { tzs, tarehe } from "@/lib/format";
import {
  ArrowLeft,
  Loader2,
  Check,
  Ban,
  X,
  User,
  FileText,
  Wallet,
  Calendar,
  Clock,
  MessageSquare,
  History,
  AlertCircle,
} from "lucide-react";

export const Route = createFileRoute("/ukaguzi-mkopo/$loanId")({
  head: () => ({
    meta: [
      { title: "Ukaguzi wa Mkopo — Money Seeking" },
      { name: "description", content: "Pitia na uidhinishe mkopo." },
    ],
  }),
  beforeLoad: () => {
    requireAuth();
    blockAdminFromPage();
  },
  component: UkaguziMkopoPage,
});

function UkaguziMkopoPage() {
  const { loanId } = Route.useParams();
  const { user } = useAuth();
  const { data: committeeCheck, isLoading: checkLoading } = useIsCommitteeMember();
  const { data: loanData, isLoading: loanLoading } = useCommitteeLoanDetail(loanId);
  const submitReview = useSubmitLoanReview();

  const [decision, setDecision] = useState<"APPROVE" | "REJECT" | null>(null);
  const [comments, setComments] = useState("");

  if (checkLoading || loanLoading) {
    return (
      <AppShell title="Ukaguzi wa Mkopo">
        <div className="flex justify-center py-12">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
        </div>
      </AppShell>
    );
  }

  if (!committeeCheck?.is_committee_member) {
    return (
      <AppShell title="Ukaguzi wa Mkopo">
        <div className="card-surface p-8 text-center">
          <AlertCircle className="mx-auto h-12 w-12 text-destructive mb-3" />
          <p className="text-lg font-semibold">Huna ruhusa</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Wanachama wa kamati ya mikopo pekee ndio wanaweza kufikia ukurasa huu.
          </p>
          <Link to="/kamati-mikopo" className="mt-4 inline-block text-sm font-semibold text-primary">
            Rudi kamati ya mikopo →
          </Link>
        </div>
      </AppShell>
    );
  }

  if (!loanData) {
    return (
      <AppShell title="Ukaguzi wa Mkopo">
        <div className="card-surface p-8 text-center text-sm text-muted-foreground">
          Mkopo haujapatikana.
        </div>
      </AppShell>
    );
  }

  const { data: loan, reviews, contributions, previous_loans, outstanding_balance } = loanData;
  // Backend now sends the known roster + who is still missing ("bado").
  // Fallback: derive from reviews when backend fields are absent (old build).
  const pendingList =
    loanData.pending_reviewers ??
    (loanData.committee_members ?? [])
      .filter(
        (m) =>
          !reviews?.some(
            (r) =>
              r.reviewer_id === m.user_id &&
              (r.decision === "APPROVE" || r.decision === "REJECT"),
          ),
      )
      .map((m) => ({ user_id: m.user_id, user_name: m.user_name ?? m.user_id }));

  // Check if current user already reviewed
  const myReview = reviews?.find((r) => r.reviewer_id === user?.id);
  const hasReviewed = myReview && myReview.decision !== "PENDING";

  const handleSubmit = async (chosen: "APPROVE" | "REJECT") => {
    setDecision(chosen);
    try {
      await submitReview.mutateAsync({
        loanId: loanId,
        data: {
          decision: chosen,
          comments: comments || undefined,
        },
      });
      setDecision(null);
      setComments("");
    } catch {
      /* handled by RQ */
    }
  };

  const approvedCount = reviews?.filter((r) => r.decision === "APPROVE").length ?? 0;
  const rejectedCount = reviews?.filter((r) => r.decision === "REJECT").length ?? 0;
  const pendingCount = pendingList.length;

  return (
    <AppShell
      title="Ukaguzi wa Mkopo"
      subtitle={`Mkopo #${loan.id} — ${loan.member?.full_name ?? "Mwanachama"}`}
      action={
        <Link
          to="/kamati-mikopo"
          className="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-2 text-sm font-semibold hover:bg-muted"
        >
          <ArrowLeft className="h-4 w-4" /> Rudi
        </Link>
      }
    >
      <div className="grid gap-5 lg:grid-cols-3">
        {/* Main content */}
        <div className="lg:col-span-2 space-y-5">
          {/* Loan Details */}
          <div className="card-surface p-5">
            <h3 className="font-display text-base font-semibold mb-4 flex items-center gap-2">
              <FileText className="h-4 w-4 text-primary" /> Taarifa za Mkopo
            </h3>

            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <p className="text-muted-foreground">Mwanachama</p>
                <p className="font-semibold">{loan.member?.full_name}</p>
                <p className="text-xs text-muted-foreground">{loan.member?.member_no}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Kiasi Kilichoombwa</p>
                <p className="font-semibold text-lg">{tzs(loan.amount)}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Madhumuni</p>
                <p className="font-semibold">{loan.purpose || "Hakuna madhumuni"}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Tarehe ya Kurudisha</p>
                <p className="font-semibold">{tarehe(loan.due_date)}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Muda wa Mkopo (siku)</p>
                <p className="font-semibold">{loan.term_days ?? "—"}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Riba</p>
                <p className="font-semibold">
                  {loan.interest_enabled
                    ? `${Number(loan.applicable_interest_rate)}% / mwezi (${loan.interest_type === "reducing" ? "salio linalopungua" : "flat"})`
                    : "Hakuna Riba"}
                </p>
              </div>
              <div>
                <p className="text-muted-foreground">Jumla ya Kurejesha</p>
                <p className="font-semibold text-lg">{tzs(Number(loan.total_repayment ?? loan.amount))}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Tarehe ya Kuomba</p>
                <p className="font-semibold">{tarehe(loan.applied_at)}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Hali ya Sasa</p>
                <span
                  className={`chip text-xs ${
                    loan.status === "PENDING"
                      ? "bg-warning/25 text-foreground"
                      : loan.status === "UNDER_REVIEW"
                      ? "bg-primary/15 text-primary"
                      : loan.status === "APPROVED"
                      ? "bg-success/15 text-success"
                      : "bg-destructive/10 text-destructive"
                  }`}
                >
                  {loan.status === "PENDING"
                    ? "Inasubiri"
                    : loan.status === "UNDER_REVIEW"
                    ? "Chini ya Ukaguzi"
                    : loan.status === "APPROVED"
                    ? "Imeidhinishwa"
                    : "Imekataliwa"}
                </span>
              </div>
            </div>
          </div>

          {/* Financial Summary */}
          <div className="card-surface p-5">
            <h3 className="font-display text-base font-semibold mb-4 flex items-center gap-2">
              <Wallet className="h-4 w-4 text-primary" /> Muhtasari wa Fedha
            </h3>

            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <p className="text-muted-foreground">Salio la Mikopo ya Sasa</p>
                <p className="font-semibold text-lg">{tzs(outstanding_balance ?? 0)}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Mikopo ya Awali</p>
                <p className="font-semibold">{previous_loans?.length ?? 0} mikopo</p>
              </div>
            </div>

            {previous_loans && previous_loans.length > 0 && (
              <div className="mt-4">
                <p className="text-xs font-medium text-muted-foreground mb-2">Mikopo ya Awali:</p>
                <div className="space-y-1.5">
                  {previous_loans.map((pl) => (
                    <div key={pl.id} className="flex items-center justify-between rounded-lg bg-muted/50 px-3 py-2 text-xs">
                      <span>Mkopo #{pl.id}</span>
                      <span>{tzs(pl.amount)}</span>
                      <span
                        className={`chip text-[10px] ${
                          pl.status === "CLOSED"
                            ? "bg-success/15 text-success"
                            : pl.status === "OUTSTANDING"
                            ? "bg-warning/25 text-foreground"
                            : "bg-muted text-muted-foreground"
                        }`}
                      >
                        {pl.status}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>

          {/* Contribution History */}
          <div className="card-surface p-5">
            <h3 className="font-display text-base font-semibold mb-4 flex items-center gap-2">
              <History className="h-4 w-4 text-primary" /> Historia ya Michango
            </h3>

            {contributions && contributions.length > 0 ? (
              <div className="space-y-1.5">
                {(contributions as any[]).slice(0, 6).map((c: any, i: number) => (
                  <div key={i} className="flex items-center justify-between rounded-lg bg-muted/50 px-3 py-2 text-xs">
                    <span>{c.month}</span>
                    <span className="font-semibold">{tzs(c.amount)}</span>
                    <span className="text-muted-foreground">{tarehe(c.paid_at)}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">Hakuna historia ya michango.</p>
            )}
          </div>

          {/* Reviews from other committee members */}
          <div className="card-surface p-5">
            <h3 className="font-display text-base font-semibold mb-4 flex items-center gap-2">
              <MessageSquare className="h-4 w-4 text-primary" /> Ukaguzi wa Kamati
            </h3>

            <div className="flex gap-4 mb-4 text-sm">
              <span className="text-success font-semibold">
                <Check className="inline h-4 w-4 mr-1" />
                {approvedCount} Wameidhinisha
              </span>
              <span className="text-destructive font-semibold">
                <Ban className="inline h-4 w-4 mr-1" />
                {rejectedCount} Wamekataa
              </span>
              <span className="text-muted-foreground font-semibold">
                <Clock className="inline h-4 w-4 mr-1" />
                {pendingCount} Bado
              </span>
            </div>

            <div className="space-y-2">
              {reviews?.map((r) => (
                <div key={r.id} className="rounded-lg bg-muted/50 px-4 py-3">
                  <div className="flex items-center justify-between">
                    <p className="font-semibold text-sm">{r.reviewer_name}</p>
                    <span
                      className={`chip text-[10px] ${
                        r.decision === "APPROVE"
                          ? "bg-success/15 text-success"
                          : r.decision === "REJECT"
                          ? "bg-destructive/10 text-destructive"
                          : "bg-muted text-muted-foreground"
                      }`}
                    >
                      {r.decision === "APPROVE"
                        ? "Imeidhinishwa"
                        : r.decision === "REJECT"
                        ? "Imekataliwa"
                        : "Inasubiri"}
                    </span>
                  </div>
                  {r.comments && (
                    <p className="mt-1 text-xs text-muted-foreground">{r.comments}</p>
                  )}
                  {r.reviewed_at && (
                    <p className="mt-1 text-[10px] text-muted-foreground">{r.reviewed_at}</p>
                  )}
                </div>
              ))}
              {pendingList.map((p) => (
                <div
                  key={`pending-${p.user_id}`}
                  className="flex items-center justify-between rounded-lg border border-dashed border-border px-4 py-3"
                >
                  <p className="font-semibold text-sm">{p.user_name}</p>
                  <span className="chip bg-muted text-muted-foreground text-[10px]">
                    Bado hajapitia
                  </span>
                </div>
              ))}
              {reviews?.length === 0 && pendingList.length === 0 && (
                <p className="text-sm text-muted-foreground">
                  Hakuna ukaguzi bado.
                </p>
              )}
            </div>
          </div>
        </div>

        {/* Sidebar — Review Action */}
        <div className="space-y-5">
          {/* Status Summary */}
          <div className="card-surface p-5">
            <h3 className="font-display text-sm font-semibold mb-3">Hali ya Ukaguzi</h3>
            <div className="space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Wanaoidhinisha</span>
                <span className="font-semibold text-success">{approvedCount}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Wanaokataa</span>
                <span className="font-semibold text-destructive">{rejectedCount}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Bado hawajapitia</span>
                <span className="font-semibold">{pendingCount}</span>
              </div>
              {pendingList.length > 0 && (
                <p className="text-xs text-muted-foreground">
                  {pendingList.map((p) => p.user_name).join(", ")}
                </p>
              )}
              <div className="h-1.5 mt-2 overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full bg-success transition-all"
                  style={{
                    width: `${
                      approvedCount + rejectedCount + pendingCount > 0
                        ? (approvedCount /
                            (approvedCount + rejectedCount + pendingCount)) *
                          100
                        : 0
                    }%`,
                  }}
                />
              </div>
            </div>
          </div>

          {/* Review Action */}
          {!hasReviewed &&
          (loan.status === "PENDING" || loan.status === "UNDER_REVIEW") ? (
            <div className="card-surface p-5">
              <h3 className="font-display text-sm font-semibold mb-3">Toa Maamuzi Yako</h3>

              <div className="space-y-3">
                <div>
                  <label className="text-sm font-medium">Maoni (si lazima)</label>
                  <textarea
                    value={comments}
                    onChange={(e) => setComments(e.target.value)}
                    placeholder="Andika maoni yako kuhusu mkopo huu..."
                    className="mt-1 w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                    rows={3}
                  />
                </div>

                {submitReview.error && (
                  <p className="text-sm text-destructive">
                    {submitReview.error.message}
                  </p>
                )}

                <div className="flex gap-2">
                  <button
                    onClick={() => handleSubmit("APPROVE")}
                    disabled={submitReview.isPending}
                    className="flex-1 inline-flex items-center justify-center gap-1.5 rounded-lg bg-success px-3 py-2.5 text-sm font-semibold text-white disabled:opacity-50"
                  >
                    {submitReview.isPending && decision === "APPROVE" ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <Check className="h-4 w-4" />
                    )}
                    Idhinisha
                  </button>
                  <button
                    onClick={() => handleSubmit("REJECT")}
                    disabled={submitReview.isPending}
                    className="flex-1 inline-flex items-center justify-center gap-1.5 rounded-lg bg-destructive px-3 py-2.5 text-sm font-semibold text-white disabled:opacity-50"
                  >
                    {submitReview.isPending && decision === "REJECT" ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <Ban className="h-4 w-4" />
                    )}
                    Kataa
                  </button>
                </div>

                <p className="text-[10px] text-muted-foreground text-center">
                  Mkopo utaidhinishwa tu pale wanachama wote wa kamati walipoidhinisha.
                  Ukataliaf utakataliwa mara moja.
                </p>
              </div>
            </div>
          ) : hasReviewed ? (
            <div className="card-surface p-5">
              <h3 className="font-display text-sm font-semibold mb-3">Ukaguzi Wako</h3>
              <div className="rounded-lg bg-muted/50 p-3">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">Maamuzi</span>
                  <span
                    className={`chip text-xs ${
                      myReview?.decision === "APPROVE"
                        ? "bg-success/15 text-success"
                        : "bg-destructive/10 text-destructive"
                    }`}
                  >
                    {myReview?.decision === "APPROVE" ? "Umeidhinisha" : "Umekataa"}
                  </span>
                </div>
                {myReview?.comments && (
                  <p className="mt-2 text-xs text-muted-foreground">
                    {myReview.comments}
                  </p>
                )}
                {myReview?.reviewed_at && (
                  <p className="mt-1 text-[10px] text-muted-foreground">
                    {myReview.reviewed_at}
                  </p>
                )}
              </div>
            </div>
          ) : null}

          {/* Member Info */}
          <div className="card-surface p-5">
            <h3 className="font-display text-sm font-semibold mb-3 flex items-center gap-2">
              <User className="h-4 w-4 text-primary" /> Taarifa za Mwanachama
            </h3>
            <div className="space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Jina</span>
                <span className="font-semibold">{loan.member?.full_name}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Namba</span>
                <span className="font-semibold">{loan.member?.member_no}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Simu</span>
                <span className="font-semibold">{loan.member?.phone}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </AppShell>
  );
}
