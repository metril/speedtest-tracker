import { describe, expect, it } from 'vitest';
import type { NotifyChannel } from '../../lib/api';
import { channelErrorTarget, isChannelUnsaved, stripIrrelevantChannelFields } from './channelHelpers';

describe('stripIrrelevantChannelFields', () => {
  it('trims apprise urls and drops blank lines', () => {
    const channel: NotifyChannel = {
      id: 'c1', type: 'apprise', name: 'phone', enabled: true, url: '',
      urls: ['  ntfy://a  ', '', '   ', 'ntfy://b'],
    };
    expect(stripIrrelevantChannelFields(channel).urls).toEqual(['ntfy://a', 'ntfy://b']);
  });

  it('handles an undefined urls list', () => {
    const channel: NotifyChannel = { id: 'c1', type: 'apprise', name: 'phone', enabled: true, url: '' };
    expect(stripIrrelevantChannelFields(channel).urls).toEqual([]);
  });
});

describe('isChannelUnsaved', () => {
  it('is true for a channel with no saved match', () => {
    const channel: NotifyChannel = { id: 'c1', type: 'webhook', name: 'hook', enabled: true, url: 'https://x' };
    expect(isChannelUnsaved(channel, [])).toBe(true);
  });

  it('is false when it matches the saved copy', () => {
    const channel: NotifyChannel = { id: 'c1', type: 'webhook', name: 'hook', enabled: true, url: 'https://x' };
    expect(isChannelUnsaved(channel, [channel])).toBe(false);
  });
});

describe('channelErrorTarget', () => {
  it('extracts a quoted name', () => {
    expect(channelErrorTarget('channel "Discord": url must be an absolute http(s) URL')).toBe('Discord');
  });

  it('extracts a bare id', () => {
    expect(channelErrorTarget('channel 4bc556d4: url must be an absolute http(s) URL')).toBe('4bc556d4');
  });

  it('returns null for a message that does not name a channel', () => {
    expect(channelErrorTarget('cooldown_minutes must be at least 1')).toBeNull();
  });
});
