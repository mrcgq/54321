<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="show"
        class="fixed inset-0 z-50 flex items-center justify-center"
        @click.self="closeOnBackdrop && $emit('close')"
      >
        <!-- 背景遮罩 -->
        <div class="absolute inset-0 bg-black/50"></div>
        
        <!-- 模态框内容 -->
        <div
          :class="[
            'relative bg-white dark:bg-gray-800 rounded-lg shadow-xl overflow-hidden',
            sizeClasses
          ]"
        >
          <!-- 标题栏 -->
          <div v-if="title" class="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
            <h3 class="text-lg font-semibold text-gray-800 dark:text-white">
              {{ title }}
            </h3>
            <button
              v-if="showClose"
              @click="$emit('close')"
              class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          
          <!-- 内容区域 -->
          <div class="p-6">
            <slot></slot>
          </div>
          
          <!-- 底部按钮区域 -->
          <div v-if="$slots.footer" class="px-6 py-4 border-t border-gray-200 dark:border-gray-700 flex justify-end gap-3">
            <slot name="footer"></slot>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'

const props = withDefaults(defineProps<{
  show: boolean
  title?: string
  size?: 'sm' | 'md' | 'lg' | 'xl'
  showClose?: boolean
  closeOnBackdrop?: boolean
  closeOnEsc?: boolean
}>(), {
  size: 'md',
  showClose: true,
  closeOnBackdrop: true,
  closeOnEsc: true
})

const emit = defineEmits<{
  close: []
}>()

const sizeClasses = computed(() => {
  const sizes = {
    sm: 'w-full max-w-sm',
    md: 'w-full max-w-md',
    lg: 'w-full max-w-lg',
    xl: 'w-full max-w-xl'
  }
  return sizes[props.size]
})

// ESC 键关闭
function handleKeydown(e: KeyboardEvent) {
  if (props.closeOnEsc && e.key === 'Escape' && props.show) {
    emit('close')
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-active > div:last-child,
.modal-leave-active > div:last-child {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from > div:last-child,
.modal-leave-to > div:last-child {
  transform: scale(0.95);
  opacity: 0;
}
</style>
