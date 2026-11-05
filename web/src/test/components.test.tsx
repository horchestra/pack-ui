import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Badge } from '../components/Badge'
import { Keys, keyStatuses, keyOrder } from '../components/Keys'
import { EventFeed } from '../components/EventFeed'
import { Diff, diffLines } from '../components/Diff'
import { mermaidOf } from '../components/Mermaid'
import type { PackEvent } from '../api'

describe('Badge', () => {
  it('colors a state by its family', () => {
    render(<Badge value="applied" />)
    expect(screen.getByText('applied')).toHaveClass('ok')
  })
  it('a missing state is a dash', () => {
    render(<Badge />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})

describe('Keys', () => {
  const events: PackEvent[] = [
    { seq: 1, time: '', type: 'plan', plan: { pack: 'p', to: '1', direction: 'install', order: ['k1', 'k2'], changes: [], steps: [], units: [] } },
    { seq: 2, time: '', type: 'node', key: 'k1', status: 'done' },
    { seq: 3, time: '', type: 'node', key: 'k2', status: 'running' },
  ]
  it('takes the plan order and the last status of a key', () => {
    expect(keyOrder(events)).toEqual(['k1', 'k2'])
    expect(keyStatuses(events).get('k2')).toBe('running')
  })
  it('renders a chip per key with its status class', () => {
    render(<Keys events={events} />)
    expect(screen.getByText('k1')).toHaveClass('done')
    expect(screen.getByText('k2')).toHaveClass('running')
  })
  it('a failed key is failed', () => {
    render(<Keys events={[{ seq: 1, time: '', type: 'node', key: 'k9', status: 'failed' }]} />)
    expect(screen.getByText('k9')).toHaveClass('failed')
  })
})

describe('EventFeed', () => {
  it('renders a line per event, a failure in red', () => {
    render(
      <EventFeed
        events={[
          { seq: 1, time: '2026-01-01T00:00:00Z', type: 'node', key: 'k1', step: 'install', status: 'done' },
          { seq: 2, time: '2026-01-01T00:00:01Z', type: 'result', error: 'boom' },
        ]}
      />,
    )
    expect(screen.getByText(/k1 install done/)).toBeInTheDocument()
    expect(screen.getByText(/failed: boom/)).toBeInTheDocument()
  })
})

describe('diffLines', () => {
  it('marks the changed lines', () => {
    const d = diffLines('a: 1\nb: 2\n', 'a: 1\nb: 3\n')
    expect(d).toEqual([
      { op: ' ', text: 'a: 1' },
      { op: '-', text: 'b: 2' },
      { op: '+', text: 'b: 3' },
      { op: ' ', text: '' },
    ])
  })
  it('a new file is all additions', () => {
    render(<Diff from="" to="x\n" />)
    expect(screen.getByTestId('diff')).toBeInTheDocument()
  })
})

describe('mermaidOf', () => {
  it('is a flowchart with an edge per wait', () => {
    const text = mermaidOf({ nodes: [{ name: 'app', waits: ['db'] }, { name: 'db' }] })
    expect(text).toContain('flowchart TD')
    expect(text).toContain('kdb --> kapp')
  })
})
