import { recordingUrl } from '../api/client'

interface AudioPlayerProps {
  path: string
}

export default function AudioPlayer({ path }: AudioPlayerProps) {
  const filename = path.split('/').pop() ?? path
  const url = recordingUrl(filename)

  return (
    <div className="flex items-center gap-2">
      <audio
        controls
        src={url}
        preload="none"
        className="h-8 w-48"
        style={{ accentColor: '#6366f1' }}
      />
    </div>
  )
}
