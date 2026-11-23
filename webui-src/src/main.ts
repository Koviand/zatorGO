import { createApp } from 'vue'
import App from './App.vue'
import { setUnauthorizedHandler } from './api/client'
import { router } from './router'
import { forceLogin } from './stores/auth'
import './styles/zatorgo.css'

setUnauthorizedHandler(forceLogin)

createApp(App).use(router).mount('#app')
