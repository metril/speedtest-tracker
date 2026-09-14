import type { NotifyChannel } from '../../lib/api';

/** Strips fields the channel's current type does not use. Called only at
 * submit time so switching types back and forth in the editor never loses
 * data (e.g. a token) before the user saves. Field-per-type here must stay
 * in sync with the fields ChannelEditor shows for each type. */
export function stripIrrelevantChannelFields(channel: NotifyChannel): NotifyChannel {
  const {
    id, type, name, enabled, url, token, headers, priority, tags, urls,
  } = channel;
  const base: NotifyChannel = {
    id, type, name, enabled, url,
  };
  switch (type) {
    case 'webhook':
      return { ...base, headers };
    case 'ntfy':
      return {
        ...base, token, priority, tags,
      };
    case 'apprise':
      return { ...base, token, tags, urls };
    default:
      return base;
  }
}

/** True when `channel` has no unsaved edits relative to the last-saved
 * server copy, i.e. it is safe to test the SAVED channel it corresponds
 * to. A channel with no matching saved copy (a new, never-saved channel)
 * is always unsaved. */
export function isChannelUnsaved(channel: NotifyChannel, saved: NotifyChannel[] | undefined): boolean {
  const match = saved?.find((c) => c.id === channel.id);
  if (!match) return true;
  return JSON.stringify(stripIrrelevantChannelFields(channel)) !== JSON.stringify(stripIrrelevantChannelFields(match));
}
