<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from './api'
type Publication = { id:string; status:string; remote_url:string; error_message:string; error_category:string }
const rows=ref<Publication[]>([]), message=ref('')
async function load(){try{rows.value=await api<Publication[]>('publications')}catch(error){message.value=error instanceof Error?error.message:String(error)}}
async function retry(publication:Publication){try{await api('publications/retry',{method:'POST',body:JSON.stringify({publication_id:publication.id})});message.value='重试完成';await load()}catch(error){message.value=`重试失败：${error instanceof Error?error.message:String(error)}`}}
onMounted(load)
</script>
<template><p v-if="message" class="notice">{{message}}</p><ul class="rows"><li v-for="publication in rows" :key="publication.id"><span>{{publication.status}} · {{publication.remote_url||publication.error_message||'待发布'}}</span><button v-if="publication.status==='failed'&&publication.error_category==='retryable'" @click="retry(publication)">重试</button></li></ul></template>
