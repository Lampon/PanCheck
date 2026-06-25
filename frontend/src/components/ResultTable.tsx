import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { PLATFORM_NAMES } from '@/utils/constants';
import { parseLink } from '@/utils/linkParser';
import type { LinkInfo } from '@/types';

interface ResultTableProps {
  invalidLinks: string[];
  lockedLinks: string[];
  pendingLinks: string[];
  validLinks: string[];
  totalDuration?: number;
  invalidFormatCount: number;
  duplicateCount: number;
}

export function ResultTable({
  invalidLinks = [],
  lockedLinks = [],
  pendingLinks = [],
  validLinks = [],
  totalDuration,
  invalidFormatCount = 0,
  duplicateCount = 0,
}: ResultTableProps) {
  const allLinks: LinkInfo[] = [
    ...validLinks.map(link => ({ link, platform: parseLink(link), status: 'valid' as const })),
    ...lockedLinks.map(link => ({ link, platform: parseLink(link), status: 'locked' as const })),
    ...invalidLinks.map(link => ({ link, platform: parseLink(link), status: 'invalid' as const })),
    ...pendingLinks.map(link => ({ link, platform: parseLink(link), status: 'pending' as const })),
  ];

  if (allLinks.length === 0) {
    return null;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>检测结果</CardTitle>
        <CardDescription>
          {totalDuration && `总耗时: ${(totalDuration / 1000).toFixed(2)}秒`}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="mb-4 space-y-2">
          <div className="flex gap-4 text-sm">
            <div className="text-green-600">
              有效: {validLinks.length}
            </div>
            <div className="text-amber-600">
              需提取码: {lockedLinks.length}
            </div>
            <div className="text-red-600">
              失效: {invalidLinks.length}
            </div>
            <div className="text-gray-600">
              待检测: {pendingLinks.length}
            </div>
          </div>
          {(invalidFormatCount > 0 || duplicateCount > 0) && (
            <div className="flex gap-4 text-sm text-amber-600">
              {duplicateCount > 0 && (
                <div>
                  重复链接: {duplicateCount}
                </div>
              )}
              {invalidFormatCount > 0 && (
                <div>
                  不规范链接: {invalidFormatCount}
                </div>
              )}
            </div>
          )}
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>链接</TableHead>
              <TableHead>平台</TableHead>
              <TableHead>状态</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {allLinks.map((linkInfo, index) => (
              <TableRow key={index}>
                <TableCell className="font-mono text-xs max-w-md truncate">
                  {linkInfo.link}
                </TableCell>
                <TableCell>
                  {PLATFORM_NAMES[linkInfo.platform] || '未知'}
                </TableCell>
                <TableCell>
                  {linkInfo.status === 'valid' && (
                    <span className="text-green-600">有效</span>
                  )}
                  {linkInfo.status === 'locked' && (
                    <span className="text-amber-600">需提取码</span>
                  )}
                  {linkInfo.status === 'invalid' && (
                    <span className="text-red-600">失效</span>
                  )}
                  {linkInfo.status === 'pending' && (
                    <span className="text-gray-600">待检测</span>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

