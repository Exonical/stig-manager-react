// Placeholder for routes that will be filled in by later 18x slices.
// Keeping the navigation alive end-to-end now lets us verify the shell
// and route guards in CI without waiting on the full feature work.


interface PlaceholderProps {
  title: string
  blurb: string
  milestone: string
}

export function Placeholder({ title, blurb, milestone }: PlaceholderProps) {
  return (
    <div className="space-y-3">
      <h1 className="text-3xl font-semibold tracking-tight">{title}</h1>
      <p className="text-sm text-[var(--color-muted-foreground)]">{blurb}</p>
      <p className="text-xs uppercase tracking-wider text-[var(--color-muted-foreground)]/80">
        Coming in milestone {milestone}.
      </p>
    </div>
  )
}
