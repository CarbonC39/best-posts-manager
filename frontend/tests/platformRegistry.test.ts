import { describe, expect, it } from 'vitest'
import { supportedBridgePlatforms } from '../src/bridgeAdapters'
import { bridgePlatformIDs, canAutoPublish, platformForBridgeHost, platformIDs, platformRegistry, userscriptMatches } from '../src/platformRegistry'

describe('platform registry', () => {
  it('has unique platform IDs', () => {
    expect(new Set(platformIDs).size).toBe(platformIDs.length)
    expect(platformRegistry.mastodon.accountSetupMode).toBe('oauth')
    expect(platformRegistry.misskey.accountSetupMode).toBe('miauth')
    expect(platformRegistry.bluesky.accountSetupMode).toBe('credential')
    expect(platformRegistry.x.accountSetupMode).toBe('bridge')
  })

  it('defines hosts and userscript matches for every bridge adapter', () => {
    expect(supportedBridgePlatforms).toEqual(bridgePlatformIDs)
    for (const platform of bridgePlatformIDs) {
      const bridge = platformRegistry[platform].bridge
      expect(bridge.hosts.length).toBeGreaterThan(0)
      expect(bridge.matches.length).toBeGreaterThan(0)
      for (const host of bridge.hosts) expect(platformForBridgeHost(host)).toBe(platform)
    }
  })

  it('keeps Rednote coverage and current client auto-publish support', () => {
    expect(platformForBridgeHost('creator.rednote.com')).toBe('xiaohongshu')
    expect(userscriptMatches).toContain('https://rednote.com/*')
    expect(userscriptMatches).toContain('https://*.rednote.com/*')
    expect(canAutoPublish('x')).toBe(true)
    expect(canAutoPublish('weibo')).toBe(true)
    expect(canAutoPublish('xiaohongshu')).toBe(false)
    expect(canAutoPublish('qzone')).toBe(false)
  })
})
