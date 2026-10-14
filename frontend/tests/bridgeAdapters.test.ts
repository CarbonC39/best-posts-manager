import { readFileSync } from 'node:fs'
import { describe, expect, it, vi } from 'vitest'
import { captureVisible, clickKnownPublish, fillVisible, fillVisibleWithMedia, supportedBridgePlatforms } from '../src/bridgeAdapters'
import { canAutoPublish, platformForBridgeHost } from '../src/platformRegistry'

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
  it('uses the visible QQ Space editor from the captured page structure', () => {
    document.body.innerHTML = '<div class="qz-poster-editor-cont"><div x:id="content_content" class="textinput textarea" contenteditable="true" style="display:none"></div><div x:id="substitutor_content" class="textinput textarea" contenteditable="true" tabindex="0">说点什么吧</div><input id="qz_app_imageReader_1" type="file"></div>'
    const substitutor = document.querySelector<HTMLElement>('[x\\:id="substitutor_content"]')!
    const activate = vi.fn()
    const content = document.querySelector<HTMLElement>('[x\\:id="content_content"]')!
    const beforeInput = vi.fn()
    const input = vi.fn()
    substitutor.addEventListener('click', activate)
    content.addEventListener('beforeinput', beforeInput)
    content.addEventListener('input', input)
    fillVisible('qzone', '空间正文')
    expect(activate).toHaveBeenCalledOnce()
    expect(content.textContent).toBe('空间正文')
    expect(beforeInput).toHaveBeenCalledOnce()
    expect(input).toHaveBeenCalledOnce()
    expect(substitutor.textContent).toBe('说点什么吧')
  })
  it('uses the current Weibo composer and its own media input', () => {
    document.body.innerHTML = '<div class="wbpro-form"><textarea placeholder="有什么新鲜事想分享给大家？"></textarea><input class="_file_hqmwy_20" type="file"></div>'
    const result = fillVisible('weibo', '微博正文')
    expect((document.querySelector('textarea') as HTMLTextAreaElement).value).toBe('微博正文')
    expect(result.mediaInput).toBe(document.querySelector('input'))
  })
  it('opens the RedNote creator text mode before filling its editor', async () => {
    document.body.innerHTML = '<button class="creator-tab">发布笔记</button><div class="image-upload-buttons"><button class="upload-button">上传图文</button><button class="text2image-button">文字配图</button></div>'
    const creatorTab = document.querySelector('.creator-tab') as HTMLButtonElement
    const textMode = document.querySelector('.text2image-button') as HTMLButtonElement
    const tabClick = vi.fn()
    const modeClick = vi.fn()
    creatorTab.addEventListener('click', tabClick)
    textMode.addEventListener('click', () => {
      modeClick()
      document.body.insertAdjacentHTML('beforeend', '<div class="ql-editor" contenteditable="true"></div>')
    })
    await fillVisibleWithMedia('xiaohongshu', { platform: 'xiaohongshu', page_url: 'https://creator.rednote.com/login?source=official', text: '小红书正文' })
    expect(tabClick).toHaveBeenCalledOnce()
    expect(modeClick).toHaveBeenCalledOnce()
    expect(document.querySelector('.ql-editor')?.textContent).toBe('小红书正文')
  })
  it('selects the RedNote upload mode when the request contains media', async () => {
    document.body.innerHTML = '<button class="creator-tab">发布笔记</button><div class="image-upload-buttons"><button class="upload-button">上传图文</button><button class="text2image-button">文字配图</button></div>'
    const uploadMode = document.querySelector('.upload-button') as HTMLButtonElement
    const uploadClick = vi.fn()
    uploadMode.addEventListener('click', () => {
      uploadClick()
      document.body.insertAdjacentHTML('beforeend', '<div class="ql-editor" contenteditable="true"></div><input type="file">')
    })
    const failures = await fillVisibleWithMedia('xiaohongshu', {
      platform: 'xiaohongshu', page_url: 'https://creator.rednote.com/login?source=official', text: '图文正文',
      media: [{ filename: 'photo.jpg', mime: 'image/jpeg', url: '/media/photo' }],
    }, async () => new Response('', { status: 404 }))
    expect(uploadClick).toHaveBeenCalledOnce()
    expect(document.querySelector('.text2image-button')).toBeTruthy()
    expect(failures[0]).toContain('photo.jpg: HTTP 404')
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
	expect(platformForBridgeHost('creator.rednote.com')).toBe('xiaohongshu')
	expect(source).toContain("addEventListener('pointerdown'")
	expect(source).toContain("if (minimized && !dragged) setMinimized(false)")
	expect(source).toContain('.bpm-shell.minimized')
	expect(canAutoPublish('x')).toBe(true)
	expect(canAutoPublish('weibo')).toBe(true)
  })
	it('auto-publishes Weibo only through one enabled visible submit control in its composer', () => {
	document.body.innerHTML = '<button>发布</button><div class="wbpro-form"><button>表情</button><button disabled>发送</button><span style="display:none"><button>发布</button></span><button>发送</button></div>'
	const publish = document.querySelectorAll<HTMLButtonElement>('.wbpro-form button')[3]
	const clicked = vi.fn()
	publish.addEventListener('click', clicked)
	expect(clickKnownPublish('weibo')).toBeNull()
	expect(clicked).toHaveBeenCalledOnce()
	document.body.innerHTML = '<div class="wbpro-form"><button>发送</button><button role="button">发布</button></div>'
	expect(clickKnownPublish('weibo')).toContain('多个发布按钮')
	document.body.innerHTML = '<div class="wbpro-form"><button>下一步</button></div>'
	expect(clickKnownPublish('weibo')).toContain('未自动点击')
	})
	it('does not fill unrelated generic textareas', () => {
	document.body.innerHTML = '<textarea placeholder="搜索或评论"></textarea>'
	expect(() => fillVisible('qzone', 'must not leak')).toThrow('未找到可见编辑框')
	expect((document.querySelector('textarea') as HTMLTextAreaElement).value).toBe('')
  })
})
