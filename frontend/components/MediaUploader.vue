<template>
  <div class="media-uploader" v-if="hasRole('citizen')">
    <div class="upload-header">
      <h3>Upload Media</h3>
      <label class="anonymous-toggle">
        <input type="checkbox" v-model="isAnonymous" />
        <span>Upload sebagai anonim</span>
      </label>
    </div>

    <div
      class="dropzone"
      :class="{ 'is-dragover': isDragover, 'has-files': files.length > 0 }"
      @drop.prevent="onDrop"
      @dragover.prevent="isDragover = true"
      @dragleave.prevent="isDragover = false"
    >
      <div v-if="files.length === 0" class="dropzone-placeholder">
        <svg class="upload-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <polyline points="17 8 12 3 7 8" />
          <line x1="12" y1="3" x2="12" y2="15" />
        </svg>
        <p>Drag & drop files atau <label for="file-input" class="browse-link">browse</label></p>
        <span class="file-types">JPG, PNG, GIF, WebP, MP4, MOV (Max 50MB)</span>
      </div>

      <div v-else class="file-preview-grid">
        <div
          v-for="(file, index) in files"
          :key="index"
          class="file-preview"
        >
          <img v-if="file.preview" :src="file.preview" :alt="file.name" />
          <div v-else class="file-icon">
            <span>{{ file.type.split('/')[0] }}</span>
          </div>
          <div class="file-info">
            <span class="file-name">{{ file.name }}</span>
            <span class="file-size">{{ formatFileSize(file.size) }}</span>
          </div>
          <button
            type="button"
            class="remove-btn"
            @click="removeFile(index)"
            :disabled="isUploading"
          >
            ×
          </button>

          <div v-if="file.uploadProgress !== undefined" class="progress-bar">
            <div class="progress-fill" :style="{ width: file.uploadProgress + '%' }"></div>
          </div>
        </div>
      </div>

      <input
        id="file-input"
        type="file"
        ref="fileInput"
        multiple
        accept=".jpg,.jpeg,.png,.gif,.webp,.mp4,.mov,.avi"
        @change="onFileSelect"
        style="display: none"
      />
    </div>

    <div v-if="uploadError" class="error-message">
      {{ uploadError }}
    </div>

    <div class="upload-actions">
      <button
        type="button"
        class="btn-secondary"
        @click="clearFiles"
        :disabled="isUploading || files.length === 0"
      >
        Clear All
      </button>
      <button
        type="button"
        class="btn-primary"
        @click="uploadAll"
        :disabled="isUploading || files.length === 0 || !reportId"
      >
        {{ isUploading ? 'Uploading...' : `Upload ${files.length} file(s)` }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{
  reportId: number | null
  maxFiles?: number
}>()

const emit = defineEmits<{
  uploaded: [response: any]
  error: [error: string]
}>()

const { hasRole } = useUser()
const { uploadFile } = useMedia()

const files = ref<Array<File & { preview?: string; uploadProgress?: number }>>([])
const isDragover = ref(false)
const isAnonymous = ref(false)
const isUploading = ref(false)
const uploadError = ref<string | null>(null)

const onDrop = (e: DragEvent) => {
  isDragover.value = false
  const droppedFiles = Array.from(e.dataTransfer?.files || [])
  addFiles(droppedFiles)
}

const onFileSelect = (e: Event) => {
  const target = e.target as HTMLInputElement
  const selectedFiles = Array.from(target.files || [])
  addFiles(selectedFiles)
}

const addFiles = (newFiles: File[]) => {
  const maxFiles = props.maxFiles || 10
  const allowedTypes = ['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'video/mp4', 'video/quicktime', 'video/x-msvideo']

  newFiles.forEach((file) => {
    if (files.value.length >= maxFiles) {
      uploadError.value = `Maximum ${maxFiles} files allowed`
      return
    }

    if (!allowedTypes.includes(file.type)) {
      uploadError.value = `File type ${file.type} not allowed`
      return
    }

    if (file.size > 50 * 1024 * 1024) {
      uploadError.value = 'File size exceeds 50MB limit'
      return
    }

    // Create preview for images
    if (file.type.startsWith('image/')) {
      const reader = new FileReader()
      reader.onload = (e) => {
        const fileWithPreview = Object.assign(file, {
          preview: e.target?.result as string
        })
        files.value.push(fileWithPreview)
      }
      reader.readAsDataURL(file)
    } else {
      files.value.push(file)
    }
  })

  uploadError.value = null
}

const removeFile = (index: number) => {
  files.value.splice(index, 1)
}

const clearFiles = () => {
  files.value = []
  uploadError.value = null
}

const uploadAll = async () => {
  if (!props.reportId) {
    uploadError.value = 'Report ID is required'
    return
  }

  isUploading.value = true
  uploadError.value = null

  const results = []

  for (let i = 0; i < files.value.length; i++) {
    const file = files.value[i]

    try {
      // Set initial progress
      file.uploadProgress = 0

      // Simulate progress
      const progressInterval = setInterval(() => {
        if (file.uploadProgress! < 90) {
          file.uploadProgress! += 10
        }
      }, 200)

      const response = await uploadFile(file, props.reportId)

      clearInterval(progressInterval)
      file.uploadProgress = 100

      if (response) {
        results.push(response)
      }
    } catch (error: any) {
      uploadError.value = error.message
      emit('error', error.message)
    }
  }

  isUploading.value = false

  if (results.length > 0) {
    emit('uploaded', results)
    clearFiles()
  }
}

const formatFileSize = (bytes: number): string => {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}
</script>

<style scoped>
.media-uploader {
  margin: 2rem 0;
}

.upload-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.upload-header h3 {
  font-family: 'Fraunces', serif;
  font-size: 1.5rem;
  color: var(--ink);
}

.anonymous-toggle {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-family: 'Space Grotesk', sans-serif;
  cursor: pointer;
}

.anonymous-toggle input[type="checkbox"] {
  width: 1.2rem;
  height: 1.2rem;
  cursor: pointer;
}

.dropzone {
  border: 2px dashed var(--sea);
  border-radius: 12px;
  padding: 2rem;
  text-align: center;
  background: var(--paper);
  transition: all 0.3s ease;
  min-height: 200px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.dropzone.is-dragover {
  border-color: var(--glow);
  background: rgba(247, 213, 138, 0.1);
  transform: scale(1.02);
}

.dropzone-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
}

.upload-icon {
  width: 64px;
  height: 64px;
  color: var(--sea);
  stroke-width: 1.5;
}

.dropzone p {
  font-family: 'Space Grotesk', sans-serif;
  font-size: 1.1rem;
  color: var(--ink);
  margin: 0;
}

.browse-link {
  color: var(--sea);
  text-decoration: underline;
  cursor: pointer;
  font-weight: 600;
}

.file-types {
  font-size: 0.9rem;
  color: #666;
}

.file-preview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 1rem;
  width: 100%;
}

.file-preview {
  position: relative;
  border: 1px solid var(--sea);
  border-radius: 8px;
  overflow: hidden;
  background: white;
}

.file-preview img {
  width: 100%;
  height: 150px;
  object-fit: cover;
}

.file-icon {
  width: 100%;
  height: 150px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--sea);
  color: white;
  font-size: 1.2rem;
  font-weight: 600;
  text-transform: uppercase;
}

.file-info {
  padding: 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.file-name {
  font-size: 0.85rem;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.file-size {
  font-size: 0.75rem;
  color: #666;
}

.remove-btn {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  background: rgba(0, 0, 0, 0.7);
  color: white;
  border: none;
  border-radius: 50%;
  width: 24px;
  height: 24px;
  cursor: pointer;
  font-size: 1.2rem;
  line-height: 1;
  transition: background 0.2s;
}

.remove-btn:hover {
  background: rgba(0, 0, 0, 0.9);
}

.progress-bar {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 4px;
  background: #e0e0e0;
}

.progress-fill {
  height: 100%;
  background: var(--glow);
  transition: width 0.3s ease;
}

.error-message {
  margin-top: 1rem;
  padding: 0.75rem;
  background: #fee;
  border: 1px solid #fcc;
  border-radius: 6px;
  color: #c00;
  font-size: 0.9rem;
}

.upload-actions {
  margin-top: 1rem;
  display: flex;
  gap: 1rem;
  justify-content: flex-end;
}

.btn-primary, .btn-secondary {
  padding: 0.75rem 1.5rem;
  border-radius: 8px;
  font-family: 'Space Grotesk', sans-serif;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
  border: none;
  font-size: 1rem;
}

.btn-primary {
  background: var(--sea);
  color: white;
}

.btn-primary:hover:not(:disabled) {
  background: #7ac0b0;
  transform: translateY(-2px);
}

.btn-secondary {
  background: transparent;
  border: 2px solid var(--ink);
  color: var(--ink);
}

.btn-secondary:hover:not(:disabled) {
  background: var(--ink);
  color: var(--paper);
}

.btn-primary:disabled, .btn-secondary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

@media (max-width: 768px) {
  .file-preview-grid {
    grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  }

  .upload-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 1rem;
  }
}
</style>
