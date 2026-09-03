import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { canAutoPublish, captureVisible, clickKnownPublish, fillVisible, supportedBridgePlatforms } from '../src/bridgeAdapters'

describe('browser bridge adapters', () => {
  it('supports the four explicit page platforms and fills visible editors', () => {
    expect(supportedBridgePlatforms).toEqual(['x', 'weibo', 'xiaohongshu', 'qzone'])
	const fixtures: Record<string, string> = {
	  x: '<div contenteditable="true" data-testid="tweetTextarea_0"></div>',
	  weibo: '<textarea placeholder="分享新鲜事"></textarea>',
	  xiaohongshu: '<section><div class="ql-editor" contenteditable="true"></div></section>',
	  qzone: '<textarea placeholder="发表说说"></textarea>',
	}
    for (const platform of supportedBridgePlatforms) {
	  document.body.innerHTML = fixtures[platform]
	  const input = document.querySelector('textarea,[contenteditable="true"]') as HTMLElement | HTMLTextAreaElement
	  let changed = false
	  input.addEventListener('input', () => { changed = true })
	  fillVisible(platform, 'visible text')
	  expect(input instanceof HTMLTextAreaElement ? input.value : input.textContent).toBe('visible text')
      expect(changed).toBe(true)
    }
  })
  it('reports an incomplete capture when no visible composer exists', () => {
    document.body.innerHTML = '<main>nothing expanded</main>'
    expect(captureVisible('qzone', 'https://qzone.qq.com/space')).toMatchObject({ incomplete: true, incomplete_reason: '当前页面没有可见正文' })
  })
  it('captures only platform-specific visible post nodes', () => {
	const fixtures = [
	  ['x', '<article><div data-testid="tweetText">x post</div><img src="https://pbs.example/x.jpg" alt="x image"></article>', 'https://x.com/post/1', 'x post'],
	  ['weibo', '<div class="WB_feed_detail"><div class="WB_text">weibo post</div></div>', 'https://weibo.com/1', 'weibo post'],
	  ['xiaohongshu', '<div class="note-container"><div class="note-content">red post</div></div>', 'https://www.xiaohongshu.com/explore/1', 'red post'],
	  ['qzone', '<div class="feed-item"><div class="feed-content">qzone post</div></div>', 'https://user.qzone.qq.com/1', 'qzone post'],
	] as const
	for (const [platform, html, url, text] of fixtures) {
	  document.body.innerHTML = html
	  expect(captureVisible(platform, url)).toMatchObject({ text, incomplete: false })
	}
	document.body.innerHTML = fixtures[0][1]
	expect(captureVisible('x', fixtures[0][2]).media[0]).toMatchObject({ alt_text: 'x image', url: 'https://pbs.example/x.jpg' })
  })
	it('auto-publishes only through an explicit request and a known visible control', () => {
	document.body.innerHTML = '<button data-testid="tweetButton" style="display:none">Hidden</button><button data-testid="tweetButton">Post</button>'
	const publish = document.querySelectorAll('button')[1] as HTMLButtonElement
	let clicked = false
	publish.addEventListener('click', () => { clicked = true })
	expect(clickKnownPublish('x')).toBeNull()
	expect(clicked).toBe(true)
	document.body.innerHTML = '<button data-testid="tweetButton">Post one</button><button data-testid="tweetButtonInline">Post two</button>'
	expect(clickKnownPublish('x')).toContain('多个发布按钮')
	document.body.innerHTML = '<button>unrelated</button>'
	expect(clickKnownPublish('x')).toContain('未自动点击')
	const source = readFileSync('src/bridgeUserscript.ts', 'utf8')
	expect(source).toContain('request.auto_publish && request.media?.length')
	expect(source).toContain('正在填入并检查发布条件')
	expect(source).toContain('fillVisibleWithMedia(platform, request')
	expect(source).toContain("attachShadow({ mode: 'closed' })")
	expect(source).toContain("const injectedOrigin = '__BPM_ORIGIN__'")
	expect(canAutoPublish('x')).toBe(true)
	expect(canAutoPublish('weibo')).toBe(false)
  })
	it('does not fill unrelated generic textareas', () => {
	document.body.innerHTML = '<textarea placeholder="搜索或评论"></textarea>'
	expect(() => fillVisible('qzone', 'must not leak')).toThrow('未找到可见编辑框')
	expect((document.querySelector('textarea') as HTMLTextAreaElement).value).toBe('')
  })
})
