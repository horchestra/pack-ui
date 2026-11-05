import { useEffect, useId, useRef, useState } from 'react'
import type { GraphResult } from '../api'
import { useTheme } from '../theme'

// Mermaid renders mermaid text (the release's graph, built from pack graph --json's nodes the way
// pack graph -f mermaid draws it). The library is heavy — it loads on first render, not with the app.
export function Mermaid({ text }: { text: string }) {
  const id = useId().replace(/[^a-zA-Z0-9]/g, '')
  const ref = useRef<HTMLDivElement>(null)
  const { theme } = useTheme()
  const [error, setError] = useState('')
  useEffect(() => {
    let live = true
    import('mermaid')
      .then(({ default: mermaid }) => {
        mermaid.initialize({ startOnLoad: false, theme: theme === 'dark' ? 'dark' : 'default' })
        return mermaid.render(`m${id}`, text)
      })
      .then(({ svg }) => {
        if (live && ref.current) ref.current.innerHTML = svg
      })
      .catch((e) => live && setError(String(e)))
    return () => {
      live = false
    }
  }, [text, id, theme])
  if (error) return <pre className="code">{text}</pre>
  return <div ref={ref} data-testid="mermaid" role="img" aria-label="graph" />
}

// mermaidOf is the graph document as mermaid text: a flowchart, an edge for every wait, savepoints bold.
export function mermaidOf(g: GraphResult): string {
  const lines = ['flowchart TD']
  for (const n of g.nodes ?? []) {
    const label = n.savepoint ? `${n.name} ⭐` : n.name
    lines.push(`  ${safe(n.name)}["${label}"]`)
  }
  for (const n of g.nodes ?? []) {
    for (const w of n.waits ?? []) {
      lines.push(`  ${safe(w)} --> ${safe(n.name)}`)
    }
  }
  return lines.join('\n')
}

const safe = (s: string) => 'k' + s.replace(/[^a-zA-Z0-9]/g, '_')
