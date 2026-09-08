<script setup lang="ts">
import { customRef, onMounted, ref } from 'vue'
import { api } from './api'
import IconButton from './IconButton.vue'

type Publication = { id:string; variant_id:string; account_id:string; status:string; remote_url:string; remote_id:string; remote_cid:string; error_message:string; error_category:string; source:string; created_at:string; published_at:string }
type Account = { id:string; name:string; platform:string }
function listOrEmpty<T>(value: T[] | null | undefined): T[] { return Array.isArray(value) ? value : [] }

const rows=ref<Publication[]>([])
const accounts=ref<Account[]>([])
const emit = defineEmits<{ notify: [message: string] }>()
const message=customRef<string>((track,trigger)=>{let value='';return{get(){track();return value},set(next){value=next;trigger();if(next)emit('notify',next)}}})
const confirmationURLs=ref<Record<string,string>>({})
const busy=ref<Set<string>>(new Set())
const deleting=ref<string>('')

const statusLabels: Record<string,string> = {
  pending:'等待处理', publishing:'正在发布', published:'已发布', failed:'发布失败',
  needs_confirmation:'需要人工确认', cancelled:'确认未发布',
}
function statusLabel(status:string){ return statusLabels[status] || status }
function accountLabel(publication:Publication){ const account=accounts.value.find(item=>item.id===publication.account_id); if(!account)return publication.account_id;const platform=({mastodon:'Mastodon','mastodon-compatible':'Mastodon',misskey:'Misskey',bluesky:'Bluesky',x:'X',weibo:'微博',xiaohongshu:'小红书',qzone:'QQ 空间'} as Record<string,string>)[account.platform]||account.platform;return `${account.name} · ${platform}` }
function setBusy(id:string,value:boolean){ const next=new Set(busy.value); if(value)next.add(id);else next.delete(id);busy.value=next }

async function load(){try{const [publications,loadedAccounts]=await Promise.all([api<Publication[]>('publications'),api<Account[]>('accounts')]);rows.value=listOrEmpty(publications);accounts.value=listOrEmpty(loadedAccounts)}catch(error){message.value=error instanceof Error?error.message:String(error)}}
async function retry(publication:Publication){if(busy.value.has(publication.id))return;setBusy(publication.id,true);try{await api('publications/retry',{method:'POST',body:JSON.stringify({publication_id:publication.id})});message.value='重试完成';await load()}catch(error){message.value=`重试失败：${error instanceof Error?error.message:String(error)}`}finally{setBusy(publication.id,false)}}
async function confirmPublished(publication:Publication){const remoteURL=(confirmationURLs.value[publication.id]||'').trim();if(!remoteURL){message.value='请填写帖子永久链接后再确认';return}setBusy(publication.id,true);try{await api(`publications/${publication.id}/confirm`,{method:'POST',body:JSON.stringify({remote_url:remoteURL})});message.value='已确认发布结果';await load()}catch(error){message.value=`确认失败：${error instanceof Error?error.message:String(error)}`}finally{setBusy(publication.id,false)}}
async function confirmNotPublished(publication:Publication){if(!window.confirm('确认这次没有发布成功？这会结束待确认状态，不会操作任何远端内容。'))return;setBusy(publication.id,true);try{await api(`publications/${publication.id}/cancel`,{method:'POST'});message.value='已记录为未发布';await load()}catch(error){message.value=`操作失败：${error instanceof Error?error.message:String(error)}`}finally{setBusy(publication.id,false)}}
async function deletePublication(publication:Publication){if(!['published','failed','cancelled'].includes(publication.status))return;if(!window.confirm('只移除本地发布记录？不会删除远端帖子。'))return;deleting.value=publication.id;try{await api(`publications/${publication.id}`,{method:'DELETE'});rows.value=rows.value.filter(item=>item.id!==publication.id);message.value='本地发布记录已移除，远端帖子未受影响'}catch(error){message.value=`移除失败：${error instanceof Error?error.message:String(error)}`}finally{deleting.value=''}}
onMounted(load)
</script>

<template>
  <div class="history-toolbar"><IconButton icon="refresh" label="刷新记录" @click="load" /></div>
  <p v-if="message" class="sr-only">{{message}}</p>
  <p v-if="rows.length===0">还没有发布记录。</p>
  <ul v-else class="publication-list">
    <li v-for="publication in rows" :key="publication.id" class="publication-card">
      <header><div><strong>{{accountLabel(publication)}}</strong><span>{{new Date(publication.created_at).toLocaleString()}}</span></div><em :class="`publication-status status-${publication.status}`">{{statusLabel(publication.status)}}</em></header>
      <a v-if="publication.remote_url" :href="publication.remote_url" target="_blank" rel="noreferrer">{{publication.remote_url}}</a>
      <span v-else-if="publication.remote_id">远端 ID：{{publication.remote_id}}</span>
      <p v-if="publication.error_message">{{publication.error_message}}</p>
      <small v-if="publication.remote_cid">CID {{publication.remote_cid}}</small>
      <div v-if="publication.status==='needs_confirmation'" class="confirmation-row">
        <input v-model="confirmationURLs[publication.id]" placeholder="帖子永久链接 https://…">
        <button class="primary-action" :disabled="busy.has(publication.id)" @click="confirmPublished(publication)">确认已发布</button>
        <button :disabled="busy.has(publication.id)" @click="confirmNotPublished(publication)">确认未发布</button>
      </div>
      <button v-if="publication.status==='failed'&&publication.error_category==='retryable'" :disabled="busy.has(publication.id)" @click="retry(publication)">重试原始内容</button>
      <IconButton v-if="['published','failed','cancelled'].includes(publication.status)" icon="trash" label="移除本地记录（不会删除远端帖子）" :disabled="deleting===publication.id" @click="deletePublication(publication)" />
    </li>
  </ul>
</template>
