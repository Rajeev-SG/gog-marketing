import { Button } from "../ui/index.js";

/** Skeleton while the first load runs; it states what is happening, not just a spinner. */
export function LoadingPanel() {
  return (
    <section className="panel loading-panel" aria-live="polite" aria-busy="true">
      <div className="loading-heading">
        <span className="spinner" aria-hidden="true" />
        <div>
          <h2>Finding your Google marketing assets…</h2>
          <p>Checking connected accounts and the access you already approved.</p>
        </div>
      </div>
      {[0, 1, 2].map((row) => (
        <div className="skeleton-row" key={row} aria-hidden="true">
          <span className="skeleton" />
          <span className="skeleton" />
          <span className="skeleton" />
        </div>
      ))}
    </section>
  );
}

export function LoadError({ onRetry }: { onRetry: () => void }) {
  return (
    <section className="panel loading-panel" role="alert">
      <h2>We couldn’t load your Google access</h2>
      <p>Your existing permissions have not changed.</p>
      <Button variant="primary" onPress={onRetry}>
        Try again
      </Button>
    </section>
  );
}
