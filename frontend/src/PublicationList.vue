<script setup lang="ts">
import { computed, customRef, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from './api'
import { cancelPublication, confirmPublication, listPublications, removePublication, retryPublication } from './api/publications'
import { publicationInProgress, publicationIsRemovable, publicationNeedsAction, publicationStatusLabel, type Publication } from './domain/publication'
import IconButton from './IconButton.vue'
import { platformLabel } from './platformRegistry'
import { ExternalLink } from '@lucide/vue'

type Account = { id:string; name:string; platform:string }
function listOrEmpty<T>(value: T[] | null | undefined): T[] { return Array.isArray(value) ? value : [] }

const props=withDefaults(defineProps<{profileId?:string}>(),{profileId:''})
const rows=ref<Publication[]>([])
const accounts=ref<Account[]>([])
const emit = defineEmits<{ notify: [message: string]; changed: [] }>()
const message=customRef<string>((track,trigger)=>{let value='';return{get(){track();return value},set(next){value=next;trigger();if(next)emit('notify',next)}}})
const confirmationURLs=ref<Record<string,string>>({})
const busy=ref<Set<string>>(new Set())
const deleting=ref<string>('')
const loadError=ref(false)
let refreshTimer:number|undefined
let loading=false
let reloadQueued=false
const selectedGroupKey=ref('action')
const groups=computed(()=>[
  {key:'action',title:'需处理',rows:rows.value.filter(item=>publicationNeedsAction(item.status))},
  {key:'progress',title:'进行中',rows:rows.value.filter(item=>publicationInProgress(item.status))},
  {key:'complete',title:'最近完成',rows:rows.value.filter(item=>!publicationNeedsAction(item.status)&&!publicationInProgress(item.status))},
].filter(group=>group.rows.length))
const activeGroup=computed(()=>groups.value.find(group=>group.key===selectedGroupKey.value)||groups.value[0])

function accountLabel(publication:Publication){ const account=accounts.value.find(item=>item.id===publication.account_id);return account?`${account.name} · ${platformLabel(account.platform)}`:publication.account_id }
function excerpt(publication:Publication){const value=(publication.text||'').trim().replace(/\s+/g,' ');return value.length>140?`${value.slice(0,140)}…`:value}
function setBusy(id:string,value:boolean){ const next=new Set(busy.value); if(value)next.add(id);else next.delete(id);busy.value=next }

async function load(silent=false){if(loading){reloadQueued=true;return}loading=true;const requestedProfileID=props.profileId;const accountQuery=requestedProfileID.trim()?`?${new URLSearchParams({profile_id:requestedProfileID.trim()})}`:'';try{const [publications,loadedAccounts]=await Promise.all([listPublications(requestedProfileID),api<Account[]>(`accounts${accountQuery}`)]);if(requestedProfileID===props.profileId){if(loadError.value)emit('notify','');loadError.value=false;rows.value=publications;accounts.value=listOrEmpty(loadedAccounts)}}catch(error){loadError.value=true;if(!silent)message.value=error instanceof Error?error.message:String(error)}finally{loading=false;if(reloadQueued){reloadQueued=false;void load(true)}}}
async function retry(publication:Publication){if(busy.value.has(publication.id))return;setBusy(publication.id,true);try{await retryPublication(publication.id);await load();emit('changed')}catch(error){message.value=`重试失败：${error instanceof Error?error.message:String(error)}`}finally{setBusy(publication.id,false)}}
async function confirmPublished(publication:Publication){const remoteURL=(confirmationURLs.value[publication.id]||'').trim();if(!remoteURL){message.value='请填写帖子永久链接后再确认';return}setBusy(publication.id,true);try{await confirmPublication(publication.id,remoteURL);message.value='已确认发布结果';await load();emit('changed')}catch(error){message.value=`确认失败：${error instanceof Error?error.message:String(error)}`}finally{setBusy(publication.id,false)}}
async function confirmNotPublished(publication:Publication){if(!window.confirm('确认这次没有发布成功？这会结束待确认状态，不会操作任何远端内容。'))return;setBusy(publication.id,true);try{await cancelPublication(publication.id);message.value='已记录为未发布';await load();emit('changed')}catch(error){message.value=`操作失败：${error instanceof Error?error.message:String(error)}`}finally{setBusy(publication.id,false)}}
async function deletePublication(publication:Publication){if(!publicationIsRemovable(publication.status))return;if(!window.confirm('只移除本地发布记录？不会删除远端帖子。'))return;deleting.value=publication.id;try{await removePublication(publication.id);rows.value=rows.value.filter(item=>item.id!==publication.id);emit('changed');message.value='本地发布记录已移除，远端帖子未受影响'}catch(error){message.value=`移除失败：${error instanceof Error?error.message:String(error)}`}finally{deleting.value=''}}
function refreshWhenVisible(){if(document.visibilityState!=='hidden')void load(true)}
onMounted(()=>{void load();refreshTimer=window.setInterval(refreshWhenVisible,3_000);window.addEventListener('focus',refreshWhenVisible);document.addEventListener('visibilitychange',refreshWhenVisible)})
onBeforeUnmount(()=>{if(refreshTimer!==undefined)window.clearInterval(refreshTimer);window.removeEventListener('focus',refreshWhenVisible);document.removeEventListener('visibilitychange',refreshWhenVisible)})
watch(()=>props.profileId,()=>void load(true))
</script>

<template>
  <p v-if="message" class="sr-only">{{message}}</p>
  <p v-if="rows.length===0 && !loadError" class="publication-empty-state">还没有发布记录。</p>
  <div class="publication-toolbar" :class="{'publication-toolbar-error':loadError}">
    <div v-if="groups.length" class="publication-filters" role="tablist" aria-label="发布状态">
      <button v-for="group in groups" :id="`publication-tab-${group.key}`" :key="group.key" type="button" role="tab" :aria-controls="`publication-panel-${group.key}`" :aria-selected="activeGroup?.key===group.key" :class="[`group-${group.key}`,{active:activeGroup?.key===group.key}]" @click="selectedGroupKey=group.key">
        <span>{{group.title}}</span><strong>{{group.rows.length}}</strong>
      </button>
    </div>
    <IconButton icon="refresh" label="刷新记录" @click="load" />
  </div>
  <section v-if="activeGroup" :id="`publication-panel-${activeGroup.key}`" :key="activeGroup.key" class="publication-panel" role="tabpanel" :aria-labelledby="`publication-tab-${activeGroup.key}`">
    <ul class="publication-list">
      <li v-for="publication in activeGroup.rows" :key="publication.id" class="publication-card">
        <header><div><a v-if="publication.topic_id" class="publication-topic" :href="`/topics/${encodeURIComponent(publication.topic_id)}`">{{publication.topic_title||'未命名话题'}}</a><strong v-else>{{publication.topic_title||accountLabel(publication)}}</strong><span>{{accountLabel(publication)}} · {{new Date(publication.created_at).toLocaleString()}}</span></div><em :class="`publication-status status-${publication.status}`">{{publicationStatusLabel(publication.status)}}</em></header>
        <p v-if="excerpt(publication)" class="publication-excerpt">{{excerpt(publication)}}</p>
        <p v-if="publication.error_message && (['failed','needs_confirmation'].includes(publication.status) || (publication.status==='cancelled' && publication.error_message!=='用户确认未发布'))" class="publication-error" :class="{'publication-cancelled-note':publication.status==='cancelled'}">{{publication.error_message}}</p>
        <small v-if="publication.remote_cid">CID {{publication.remote_cid}}</small>
        <div v-if="publication.status==='needs_confirmation'" class="confirmation-row">
          <input v-model="confirmationURLs[publication.id]" placeholder="帖子永久链接 https://…">
          <button class="primary-action" :disabled="busy.has(publication.id)" @click="confirmPublished(publication)">确认已发布</button>
          <button :disabled="busy.has(publication.id)" @click="confirmNotPublished(publication)">确认未发布</button>
        </div>
        <footer v-if="publication.remote_url || publication.remote_id || (publication.status==='failed'&&publication.error_category==='retryable') || ['published','failed','cancelled'].includes(publication.status)" class="publication-card-actions">
          <a v-if="publication.remote_url" class="publication-remote-link" :href="publication.remote_url" target="_blank" rel="noreferrer"><ExternalLink aria-hidden="true"/><span>查看帖子</span></a>
          <span v-else-if="publication.remote_id" class="publication-remote-id">远端 ID：{{publication.remote_id}}</span>
          <button v-if="publication.status==='failed'&&publication.error_category==='retryable'" :disabled="busy.has(publication.id)" @click="retry(publication)">重试原始内容</button>
          <IconButton v-if="['published','failed','cancelled'].includes(publication.status)" icon="trash" label="移除本地记录（不会删除远端帖子）" :disabled="deleting===publication.id" @click="deletePublication(publication)" />
        </footer>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.history-page, .publication-panel, .publication-list, .publication-card { min-width: 0; max-width: 100%; box-sizing: border-box; }
.publication-card { display: grid; gap: 9px; padding: 13px 14px; border: 1px solid #dce4ea; border-radius: 12px; color: #253548; background: #fff; }
.publication-card header { display: flex; min-width: 0; align-items: flex-start; justify-content: space-between; gap: 10px; }
.publication-card header > div { min-width: 0; }
.publication-card header strong, .publication-card header span { display: block; min-width: 0; overflow-wrap: anywhere; }
.publication-card header span { margin-top: 4px; color: #56687a; font-size: 12px; }
.publication-card .publication-excerpt { margin: 0; color: #253548; font-size: 14px; line-height: 1.5; overflow-wrap: anywhere; }
.publication-card .publication-error { margin: 0; color: #913d3d; font-size: 13px; line-height: 1.45; overflow-wrap: anywhere; }
.publication-card .publication-cancelled-note { color: #56687a; }
.publication-card .publication-status { flex: 0 0 auto; padding: 4px 8px; border-radius: 8px; color: #445a6d; background: #eef2f5; font-size: 12px; line-height: 1.25; }
.publication-card .status-published { color: #286344; background: #eaf4ed; }
.publication-card .status-needs_confirmation { color: #7b581e; background: #faf3e6; }
.publication-card .status-failed { color: #913d3d; background: #faefed; }
.publication-card .status-cancelled { color: #56687a; background: #f0f3f5; }
.publication-card-actions { display: flex; min-width: 0; align-items: center; justify-content: space-between; gap: 8px; }
.publication-remote-id { min-width: 0; color: #56687a; font-size: 12px; overflow-wrap: anywhere; }
.publication-remote-link { display: inline-flex; width: fit-content; max-width: 100%; align-items: center; gap: 6px; color: #425b70; font-size: 13px; overflow-wrap: anywhere; }
.publication-remote-link svg { flex: 0 0 16px; width: 16px; height: 16px; }
.publication-card small { color: #56687a; font-size: 12px; overflow-wrap: anywhere; }
.publication-card-actions .icon-button { flex: 0 0 36px; margin-left: auto; }
.publication-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin: 22px 0 18px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--preview-line, #dedfd9);
}

.publication-filters {
  display: flex;
  align-items: center;
  gap: 4px;
}

.publication-filters button {
  --group-color: #69736b;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 36px;
  padding: 7px 10px;
  border: 1px solid transparent;
  border-radius: 7px;
  color: #5f675f;
  background: transparent;
  font-size: 13px;
}

.publication-filters .group-action { --group-color: #a05a50; }
.publication-filters .group-progress { --group-color: #98733c; }
.publication-filters .group-complete { --group-color: #52745c; }

.publication-filters button:hover {
  background: #f4f6f2;
}

.publication-filters button.active {
  border-color: #cbd7cd;
  color: var(--preview-ink, #283129);
  background: #edf3ee;
}

.publication-filters .group-action.active { border-color: #e5c9c3; color: #864c45; background: #faefed; }
.publication-filters .group-progress.active { border-color: #d8d4c7; color: #6f5d36; background: #faf6e9; }
.publication-filters .group-complete.active { border-color: #cbd7cd; color: #486c52; background: #edf4ee; }

.publication-filters strong {
  min-width: 20px;
  padding: 1px 6px;
  border-radius: 10px;
  color: #fff;
  background: var(--group-color);
  font-size: 12px;
  line-height: 18px;
  text-align: center;
}

.publication-panel .publication-list {
  margin-top: 0;
}

.publication-empty-state { display: flex; min-height: 44px; align-items: center; justify-content: space-between; gap: 12px; margin: 0; color: #56687a; font-size: 14px; }

@media (max-width: 620px) {
  .publication-toolbar { align-items: flex-start; }
  .publication-toolbar-error { margin-top: 70px; }
  .publication-filters { flex-wrap: wrap; }
  .publication-card { padding: 11px 12px; }
  .publication-card header { align-items: flex-start; }
  .publication-card .publication-status { max-width: 108px; white-space: normal; text-align: center; }
}
</style>
