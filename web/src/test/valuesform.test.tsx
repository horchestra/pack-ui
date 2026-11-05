import { describe, expect, it } from 'vitest'
import { useState } from 'react'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import Form from '@rjsf/core'
import validator from '@rjsf/validator-ajv8'
import { pointerGet, pointerSet } from '../pages/Values'

// The form a schema renders (C3): enum is a select, pattern and bounds refuse, a computed field is marked.
// The fixture schema is shaped as the values schema of examples/postgres comes out of `pack values schema
// --json` — the x-pack-* extensions kept.
const fixture = {
  type: 'object',
  properties: {
    postgres: {
      type: 'object',
      title: 'Postgres',
      properties: {
        pg_port: {
          type: 'integer',
          title: 'Port',
          minimum: 1,
          maximum: 65535,
          default: 5432,
          examples: [5433],
        },
        log_level: {
          type: 'string',
          title: 'Log level',
          enum: ['trace', 'debug', 'info', 'warn', 'error'],
          default: 'info',
        },
        id: {
          type: 'string',
          title: 'ID',
          pattern: '^[a-z0-9_]*$',
          maxLength: 63,
        },
        storage: {
          type: 'object',
          title: 'Storage',
          'x-pack-compute': 'storage of the installation',
          properties: { db: { type: 'string' } },
        },
        password: {
          type: 'object',
          title: 'Password',
          format: 'password',
        },
      },
      required: ['pg_port'],
    },
  },
} as const

describe('pointerGet/pointerSet', () => {
  it('walks and sets a JSON Pointer, immutably', () => {
    const doc = { postgres: { pg_port: 5432 } }
    expect(pointerGet(doc, '/postgres/pg_port')).toBe(5432)
    const next = pointerSet(doc, '/postgres/log_level', 'debug') as typeof doc
    expect(pointerGet(next, '/postgres/log_level')).toBe('debug')
    expect(pointerGet(next, '/postgres/pg_port')).toBe(5432)
    expect(pointerGet(doc, '/postgres/log_level')).toBeUndefined()
  })
  it('makes the path it does not find', () => {
    const next = pointerSet({}, '/a/b/c', 1)
    expect(pointerGet(next, '/a/b/c')).toBe(1)
  })
})

describe('the schema form', () => {
  it('renders a field per setting, an enum as a select, a default in place', () => {
    render(<Form schema={fixture as never} validator={validator} formData={{}} onChange={() => {}} children={<div />} />)
    expect(screen.getByText('Port')).toBeInTheDocument()
    const select = screen.getByLabelText(/Log level/) as HTMLSelectElement
    expect(select.tagName).toBe('SELECT')
    expect([...select.options].map((o) => o.label)).toContain('debug')
  })

  it('refuses a value off the pattern and out of the bounds', async () => {
    function Live() {
      const [data, setData] = useState<unknown>({})
      return (
        <Form
          schema={fixture as never}
          validator={validator}
          formData={data}
          liveValidate="onChange"
          onChange={(e) => setData(e.formData)}
        />
      )
    }
    render(<Live />)
    fireEvent.change(screen.getByLabelText(/ID/), { target: { value: 'BAD!' } })
    fireEvent.change(screen.getByLabelText(/Port/), { target: { value: '70000' } })
    fireEvent.click(screen.getByRole('button', { name: /submit/i }))
    await waitFor(() => {
      expect(screen.getAllByText(/pattern/).length).toBeGreaterThan(0)
      expect(screen.getAllByText(/65535/).length).toBeGreaterThan(0)
    })
  })

  it('marks a required field', () => {
    render(<Form schema={fixture as never} validator={validator} formData={{}} onChange={() => {}} children={<div />} />)
    expect(screen.getByText('Port').parentElement?.textContent).toContain('*')
  })
})
