import type { MediaUploadResponse } from '~/types'

export const useMedia = () => {
  const { token } = useAuth()
  const uploadProgress = ref(0)
  const isUploading = ref(false)
  const uploadError = ref<string | null>(null)

  const uploadFile = async (
    file: File,
    reportId: number
  ): Promise<MediaUploadResponse | null> => {
    console.log('[useMedia] uploadFile called:', { fileName: file.name, reportId, fileSize: file.size });
    isUploading.value = true
    uploadProgress.value = 0
    uploadError.value = null

    try {
      const formData = new FormData()
      formData.append('file', file)
      formData.append('report_id', reportId.toString())

      console.log('[useMedia] FormData prepared, calling /upload');
      console.log('[useMedia] Token present:', !!token.value);

      // Simulate progress (media service doesn't support real progress yet)
      const progressInterval = setInterval(() => {
        if (uploadProgress.value < 90) {
          uploadProgress.value += 10
        }
      }, 200)

      const response = await $fetch<MediaUploadResponse>(
        '/upload',
        {
          method: 'POST',
          body: formData,
          headers: token.value ? {
            Authorization: `Bearer ${token.value}`
          } : undefined
        }
      )

      console.log('[useMedia] Upload response:', response);
      clearInterval(progressInterval)
      uploadProgress.value = 100

      return response
    } catch (error: any) {
      console.error('[useMedia] Upload error:', error);
      uploadError.value = error.data?.error || error.message || 'Upload failed'
      return null
    } finally {
      isUploading.value = false
    }
  }

  return {
    uploadFile,
    uploadProgress,
    isUploading,
    uploadError
  }
}
