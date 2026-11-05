// Json renders a pack document the SPA has no screen of its own for: pretty-printed, collapsible nowhere —
// the document is the UI's contract, the viewer is honest about it.
export function Json({ doc }: { doc: unknown }) {
  return <pre className="code" data-testid="json">{JSON.stringify(doc, null, 2)}</pre>
}
