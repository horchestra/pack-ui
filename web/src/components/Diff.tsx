// A unified line diff of two texts — the rendered template against the installed file (LCS on lines).

export interface DiffLine {
  op: ' ' | '+' | '-'
  text: string
}

export function diffLines(a: string, b: string): DiffLine[] {
  const x = a.split('\n')
  const y = b.split('\n')
  const m = x.length
  const n = y.length
  // the LCS table, then the walk back
  const dp: number[][] = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0))
  for (let i = m - 1; i >= 0; i--) {
    for (let j = n - 1; j >= 0; j--) {
      dp[i][j] = x[i] === y[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }
  const out: DiffLine[] = []
  let i = 0
  let j = 0
  while (i < m && j < n) {
    if (x[i] === y[j]) {
      out.push({ op: ' ', text: x[i] })
      i++
      j++
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      out.push({ op: '-', text: x[i] })
      i++
    } else {
      out.push({ op: '+', text: y[j] })
      j++
    }
  }
  for (; i < m; i++) out.push({ op: '-', text: x[i] })
  for (; j < n; j++) out.push({ op: '+', text: y[j] })
  return out
}

export function Diff({ from, to }: { from: string; to: string }) {
  const lines = diffLines(from, to)
  return (
    <pre className="code" data-testid="diff">
      {lines.map((l, i) => (
        <div key={i} className={l.op === '+' ? 'diff-add' : l.op === '-' ? 'diff-del' : undefined}>
          {l.op} {l.text}
        </div>
      ))}
    </pre>
  )
}
