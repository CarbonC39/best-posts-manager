export type AccountSetupMode = 'oauth' | 'miauth' | 'credential' | 'bridge'
export type ClientAutoPublish = 'none' | 'text-only'

type BridgeDefinition = {
  hosts: readonly string[]
  matches: readonly string[]
  clientAutoPublish: ClientAutoPublish
}

type PlatformDefinition = {
  label: string
  accountSetupMode: AccountSetupMode
  setupDescription: string
  bridge?: BridgeDefinition
}

export const platformRegistry = {
  mastodon: {
    label: 'Mastodon',
    accountSetupMode: 'oauth',
    setupDescription: 'Mastodon：推荐 OAuth 自动连接；也可手动保存 access token，或使用环境变量。',
  },
  'mastodon-compatible': {
    label: 'Mastodon',
    accountSetupMode: 'credential',
    setupDescription: 'Mastodon：推荐 OAuth 自动连接；也可手动保存 access token，或使用环境变量。',
  },
  misskey: {
    label: 'Misskey',
    accountSetupMode: 'miauth',
    setupDescription: 'Misskey：推荐 MiAuth 自动连接；也可手动保存 access token，或使用环境变量。',
  },
  bluesky: {
    label: 'Bluesky',
    accountSetupMode: 'credential',
    setupDescription: 'Bluesky：填写 handle/identifier、PDS（默认 https://bsky.social）与 app password。app password 可直接保存在本机凭据文件，也可使用环境变量。',
  },
  x: {
    label: 'X',
    accountSetupMode: 'bridge',
    setupDescription: 'X：使用浏览器桥接；默认只填入，纯文本可对单次请求确认自动发布。',
    bridge: {
      hosts: ['x.com', 'twitter.com'],
      matches: ['https://x.com/*', 'https://*.x.com/*', 'https://twitter.com/*', 'https://*.twitter.com/*'],
      clientAutoPublish: 'text-only',
    },
  },
  weibo: {
    label: '微博',
    accountSetupMode: 'bridge',
    setupDescription: '微博：使用浏览器桥接填入；纯文本可在最终确认时选择自动发布，带图片仍需人工确认。',
    bridge: {
      hosts: ['weibo.com'],
      matches: ['https://weibo.com/*', 'https://*.weibo.com/*'],
      clientAutoPublish: 'text-only',
    },
  },
  xiaohongshu: {
    label: '小红书',
    accountSetupMode: 'bridge',
    setupDescription: '小红书：使用浏览器桥接填入；发布按钮选择器尚未安全验收，因此最终发布保持人工操作。',
    bridge: {
      hosts: ['xiaohongshu.com', 'rednote.com'],
      matches: ['https://xiaohongshu.com/*', 'https://*.xiaohongshu.com/*', 'https://rednote.com/*', 'https://*.rednote.com/*'],
      clientAutoPublish: 'none',
    },
  },
  qzone: {
    label: 'QQ 空间',
    accountSetupMode: 'bridge',
    setupDescription: 'QQ空间：使用浏览器桥接填入；可见范围、发布和图片顺序仍需人工检查。',
    bridge: {
      hosts: ['qzone.qq.com'],
      matches: ['https://qzone.qq.com/*', 'https://*.qzone.qq.com/*'],
      clientAutoPublish: 'none',
    },
  },
} as const satisfies Record<string, PlatformDefinition>

export type PlatformID = keyof typeof platformRegistry
export type BridgePlatformID = {
  [ID in PlatformID]: typeof platformRegistry[ID] extends { bridge: BridgeDefinition } ? ID : never
}[PlatformID]

export const platformIDs = Object.keys(platformRegistry) as PlatformID[]
export const bridgePlatformIDs = platformIDs.filter((platform): platform is BridgePlatformID => 'bridge' in platformRegistry[platform])
export const userscriptMatches = bridgePlatformIDs.flatMap((platform) => platformRegistry[platform].bridge.matches)

function bridgeDefinition(platform: string): BridgeDefinition | undefined {
  const definition = platformRegistry[platform as PlatformID]
  return definition && 'bridge' in definition ? definition.bridge : undefined
}

export function platformLabel(platform?: string) {
  return platformRegistry[platform as PlatformID]?.label || platform || '平台'
}

export function platformSetupDescription(platform: string) {
  return platformRegistry[platform as PlatformID]?.setupDescription || ''
}

export function isBridgePlatform(platform: string): platform is BridgePlatformID {
  return Boolean(bridgeDefinition(platform))
}

export function canAutoPublish(platform: string) {
  return bridgeDefinition(platform)?.clientAutoPublish === 'text-only'
}

export function platformForBridgeHost(hostname: string): BridgePlatformID | null {
  const host = hostname.toLowerCase()
  return bridgePlatformIDs.find((platform) => platformRegistry[platform].bridge.hosts.some((domain) => host === domain || host.endsWith(`.${domain}`))) || null
}
