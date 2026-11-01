import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  normalizePublicStats,
  isPublicStatsEmpty,
  formatCount,
  topChannelsWindowLabel,
  MAX_TOP_CHANNELS,
  MAX_TOP_FISHERS,
} from '../app/utils/publicStats.ts'

const sample = {
  active_chats: 12,
  active_chats_definition: 'Channels the bot is serving.',
  top_channels: [{ display_name: 'TimAllenFanClubOfficial', uses: 1234 }],
  top_channels_window: null,
  top_channels_window_days: 30,
  top_fishers: [{ display_name: 'somechatter', score: 5230, catches: 87 }],
  generated_at: '2026-10-31T21:18:26Z',
}

test('normalizes the documented contract unchanged', () => {
  assert.deepEqual(normalizePublicStats(sample), sample)
})

test('non-object payloads yield null', () => {
  for (const raw of [null, undefined, 'oops', 42, [], true]) {
    assert.equal(normalizePublicStats(raw), null)
  }
})

test('error body (503 shape) degrades to empty stats', () => {
  const s = normalizePublicStats({ error: 'stats temporarily unavailable' })
  assert.ok(s)
  assert.equal(s.active_chats, null)
  assert.deepEqual(s.top_channels, [])
  assert.deepEqual(s.top_fishers, [])
  assert.equal(s.generated_at, null)
  assert.equal(isPublicStatsEmpty(s), true)
})

test('null lists are treated as empty', () => {
  const s = normalizePublicStats({ ...sample, top_channels: null, top_fishers: null })
  assert.ok(s)
  assert.deepEqual(s.top_channels, [])
  assert.deepEqual(s.top_fishers, [])
  assert.equal(isPublicStatsEmpty(s), false) // active_chats still 12
})

test('caps list lengths', () => {
  const channels = Array.from({ length: 20 }, (_, i) => ({ display_name: `c${i}`, uses: 100 - i }))
  const fishers = Array.from({ length: 20 }, (_, i) => ({
    display_name: `f${i}`,
    score: 100 - i,
    catches: i,
  }))
  const s = normalizePublicStats({ ...sample, top_channels: channels, top_fishers: fishers })
  assert.ok(s)
  assert.equal(s.top_channels.length, MAX_TOP_CHANNELS)
  assert.equal(s.top_fishers.length, MAX_TOP_FISHERS)
  assert.equal(s.top_channels[0].display_name, 'c0')
})

test('drops malformed entries and bad numbers', () => {
  const s = normalizePublicStats({
    ...sample,
    active_chats: -3,
    top_channels: [
      null,
      'x',
      { display_name: '', uses: 5 },
      { display_name: 'ok', uses: 'many' },
      { display_name: 'nan', uses: Number.NaN },
      { display_name: '  good  ', uses: 7.9 },
    ],
    top_fishers: [
      { display_name: 42, score: 1 },
      { display_name: 'fish', score: 3 },
    ],
    top_channels_window_days: 0,
    generated_at: 'not a date',
  })
  assert.ok(s)
  assert.equal(s.active_chats, null)
  assert.deepEqual(s.top_channels, [{ display_name: 'good', uses: 7 }])
  assert.deepEqual(s.top_fishers, [{ display_name: 'fish', score: 3, catches: 0 }])
  assert.equal(s.top_channels_window_days, null)
  assert.equal(s.generated_at, null)
})

test('truncates overly long names and definitions', () => {
  const s = normalizePublicStats({
    ...sample,
    active_chats_definition: 'd'.repeat(2000),
    top_channels: [{ display_name: 'n'.repeat(300), uses: 1 }],
  })
  assert.ok(s)
  assert.equal(s.top_channels[0].display_name.length, 64)
  assert.equal(s.active_chats_definition.length, 500)
})

test('markup in names is kept as plain text (rendered escaped by Vue)', () => {
  const s = normalizePublicStats({
    ...sample,
    top_fishers: [{ display_name: '<img src=x>', score: 1, catches: 1 }],
  })
  assert.ok(s)
  assert.equal(s.top_fishers[0].display_name, '<img src=x>')
})

test('zero active chats with no lists is empty', () => {
  const s = normalizePublicStats({ ...sample, active_chats: 0, top_channels: [], top_fishers: [] })
  assert.ok(s)
  assert.equal(isPublicStatsEmpty(s), true)
})

test('formatCount groups thousands', () => {
  assert.equal(formatCount(1234567), '1,234,567')
  assert.equal(formatCount(0), '0')
})

test('all-time window (issue #184)', () => {
  const s = normalizePublicStats({
    ...sample,
    top_channels_window: 'all_time',
    top_channels_window_days: 0,
  })
  assert.ok(s)
  assert.equal(s.top_channels_window, 'all_time')
  assert.equal(s.top_channels_window_days, null)
  const bogus = normalizePublicStats({ ...sample, top_channels_window: '<script>' })
  assert.ok(bogus)
  assert.equal(bogus.top_channels_window, null)
})

test('window label: all_time wins over any days value', () => {
  for (const days of [0, 30]) {
    const s = normalizePublicStats({
      ...sample,
      top_channels_window: 'all_time',
      top_channels_window_days: days,
    })
    assert.ok(s)
    assert.equal(topChannelsWindowLabel(s), 'all time')
  }
})

test('window label: missing or zero days without a window is all time', () => {
  for (const days of [undefined, null, 0, -5, 'x']) {
    const s = normalizePublicStats({
      ...sample,
      top_channels_window: undefined,
      top_channels_window_days: days,
    })
    assert.ok(s)
    assert.equal(topChannelsWindowLabel(s), 'all time')
  }
  const err = normalizePublicStats({ error: 'x' })
  assert.ok(err)
  assert.equal(topChannelsWindowLabel(err), 'all time')
})

test('window label: positive days (legacy payload) keeps "last N days"', () => {
  const s = normalizePublicStats({ ...sample, top_channels_window_days: 30 })
  assert.ok(s)
  assert.equal(topChannelsWindowLabel(s), 'last 30 days')
  const one = normalizePublicStats({ ...sample, top_channels_window_days: 1 })
  assert.ok(one)
  assert.equal(topChannelsWindowLabel(one), 'last 1 day')
})
