import { CircleHelp } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';

/** HintTip renders a small help icon that shows explanatory text in a
 * tooltip, used next to a label instead of a permanent paragraph of hint
 * text below the control. */
export function HintTip({ hint }: { hint: string }) {
  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          <button type="button" aria-label={hint} className="inline-flex text-faint hover:text-fg">
            <CircleHelp className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent className="max-w-xs">{hint}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
