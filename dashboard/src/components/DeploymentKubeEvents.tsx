import { ScrollFollow } from 'react-lazylog'
import { formatTime } from '@/utils/datetime'
import useTranslation from '@/hooks/useTranslation'
import { IWsRespSchema } from '@/schemas/websocket'
import { IKubeEventSchema } from '@/schemas/kube_event'
import qs from 'qs'
import { useEffect, useRef, useState } from 'react'
import { toaster } from 'baseui/toast'
import LazyLog from './LazyLog'

interface IDeploymentKubeEventsProps {
    orgName: string
    clusterName: string
    deploymentName: string
    podName?: string
    open?: boolean
    width?: number | 'auto'
    height?: number | string
}

export default function DeploymentKubeEvents({
    orgName,
    clusterName,
    deploymentName,
    podName,
    open,
    width,
    height,
}: IDeploymentKubeEventsProps) {
    const wsUrl = `${window.location.protocol === 'http:' ? 'ws:' : 'wss:'}//${
        window.location.host
    }/ws/v1/orgs/${orgName}/clusters/${clusterName}/deployments/${deploymentName}/kube_events${qs.stringify(
        {
            pod_name: podName,
        },
        {
            addQueryPrefix: true,
        }
    )}`

    const [t] = useTranslation()

    const [items, setItems] = useState<string[]>([])
    const wsRef = useRef(null as null | WebSocket)
    const wsOpenRef = useRef(false)
    const selfCloseRef = useRef(false)

    useEffect(() => {
        if (!open) {
            return undefined
        }
        let ws: WebSocket | undefined
        const connect = () => {
            ws = new WebSocket(wsUrl)
            selfCloseRef.current = false
            ws.onmessage = (e) => {
                const resp = JSON.parse(e.data) as IWsRespSchema<IKubeEventSchema[]>
                if (resp.type !== 'success') {
                    toaster.negative(resp.message, {})
                    return
                }
                const events = resp.payload
                if (events.length === 0) {
                    setItems([t('no event')])
                    return
                }
                setItems(
                    events.map((event) => {
                        if (podName) {
                            return `[${event.lastTimestamp ? formatTime(event.lastTimestamp) : '-'}] [${
                                event.reason
                            }] ${event.message}`
                        }
                        return `[${event.lastTimestamp ? formatTime(event.lastTimestamp) : '-'}] [${
                            event.involvedObject?.kind ?? '-'
                        }] [${event.involvedObject?.name ?? '-'}] [${event.reason}] ${event.message}`
                    })
                )
            }
            ws.onopen = () => {
                wsOpenRef.current = true
                if (ws) {
                    wsRef.current = ws
                }
            }
            ws.onclose = () => {
                wsOpenRef.current = false
                if (selfCloseRef.current) {
                    return
                }
                connect()
            }
        }
        connect()
        return () => {
            ws?.close()
            selfCloseRef.current = true
            wsRef.current = null
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [wsUrl, open])

    return (
        <div style={{ height }}>
            <ScrollFollow
                startFollowing
                render={({ follow }) => (
                    <LazyLog
                        caseInsensitive
                        enableSearch
                        selectableLines
                        width={width}
                        text={items.length > 0 ? items.join('\n') : ' '}
                        follow={follow}
                    />
                )}
            />
        </div>
    )
}
