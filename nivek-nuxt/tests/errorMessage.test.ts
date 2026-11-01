import { test } from 'node:test'
import assert from 'node:assert/strict'
import { errorMessage } from '../app/utils/errorMessage.ts'

const FALLBACK = "Couldn't load things."

test('4xx with server error text shows it', () => {
  assert.equal(errorMessage({ statusCode: 400, data: { error: 'trigger is required' } }, FALLBACK), 'trigger is required')
  assert.equal(errorMessage({ status: 409, data: { error: 'already exists' } }, FALLBACK), 'already exists')
  assert.equal(errorMessage({ response: { status: 422 }, data: { error: 'bad input' } }, FALLBACK), 'bad input')
})

test('4xx falls back to data.message, then the fallback', () => {
  assert.equal(errorMessage({ statusCode: 400, data: { message: 'nope' } }, FALLBACK), 'nope')
  assert.equal(errorMessage({ statusCode: 400, data: { error: '   ' } }, FALLBACK), FALLBACK)
  assert.equal(errorMessage({ statusCode: 400, data: { error: 42 } }, FALLBACK), FALLBACK)
  assert.equal(errorMessage({ statusCode: 404 }, FALLBACK), FALLBACK)
})

test('5xx never shows server text', () => {
  assert.equal(errorMessage({ statusCode: 500, data: { error: 'pq: connection refused' } }, FALLBACK), FALLBACK)
  assert.equal(errorMessage({ statusCode: 502, data: '<html>bad gateway</html>' }, FALLBACK), FALLBACK)
})

test('network errors and non-objects use the fallback, not err.message', () => {
  assert.equal(errorMessage(new TypeError('Failed to fetch'), FALLBACK), FALLBACK)
  assert.equal(errorMessage({ message: '[GET] "/api/x": <no response> fetch failed' }, FALLBACK), FALLBACK)
  assert.equal(errorMessage(undefined, FALLBACK), FALLBACK)
  assert.equal(errorMessage(null, FALLBACK), FALLBACK)
  assert.equal(errorMessage('boom', FALLBACK), FALLBACK)
})
